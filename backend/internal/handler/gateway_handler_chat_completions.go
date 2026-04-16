package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ChatCompletions handles OpenAI Chat Completions API endpoint for Anthropic platform groups.
// POST /v1/chat/completions
// This converts Chat Completions requests to Anthropic format (via Responses format chain),
// forwards to Anthropic upstream, and converts responses back to Chat Completions format.
func (h *GatewayHandler) ChatCompletions(c *gin.Context) {
	streamStarted := false

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.chat_completions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// Read request body
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.chatCompletionsErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	// Validate JSON
	if !gjson.ValidBytes(body) {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model and stream
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	reqStream := gjson.GetBytes(body, "stream").Bool()
	queueFirstBilling := queueFirstNonStreamEnabled(h.cfg, reqStream)
	streamingBillingV2 := streamingV2Enabled(h.cfg, reqStream)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// Claude Code only restriction
	if apiKey.Group != nil && apiKey.Group.ClaudeCodeOnly {
		h.chatCompletionsErrorResponse(c, http.StatusForbidden, "permission_error",
			"This group is restricted to Claude Code clients (/v1/messages only)")
		return
	}

	// Error passthrough binding
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	// 1. Acquire user concurrency slot
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		reqLog.Warn("gateway.cc.user_wait_counter_increment_failed", zap.Error(err))
	} else if !canWait {
		h.chatCompletionsErrorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
	}
	if err == nil && canWait {
		waitCounted = true
	}
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		}
	}()

	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		reqLog.Warn("gateway.cc.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		waitCounted = false
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. Re-check billing
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("gateway.cc.billing_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.chatCompletionsErrorResponse(c, status, code, message)
		return
	}

	// Parse request for session hash
	parsedReq, _ := service.ParseGatewayRequest(body, "chat_completions")
	if parsedReq == nil {
		parsedReq = &service.ParsedRequest{Model: reqModel, Stream: reqStream, Body: body}
	}
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 3. Account selection + failover loop
	fs := NewFailoverState(h.maxAccountSwitches, false)
	streamReservationID := ""
	streamReservationPublished := false
	streamReservationFinalized := false
	var streamReservationAccount *service.Account
	var terminalCapture *streamingTerminalCapture
	releaseStreamReservation := func(reason string) {
		if !streamingBillingV2 || !streamReservationPublished || streamReservationFinalized {
			return
		}
		if terminalCapture != nil {
			terminalCapture.DiscardTerminal(c)
			terminalCapture = nil
		}
		acct := streamReservationAccount
		if err := h.executeUsageRecordTask(func(ctx context.Context) error {
			return h.gatewayService.PublishStreamingRelease(ctx, &service.StreamingBillingLifecycleInput{
				RequestID:          streamReservationID,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            acct,
				Subscription:       subscription,
				Model:              reqModel,
				RequestPayloadHash: requestPayloadHash,
			})
		}); err != nil {
			reqLog.Error("gateway.cc.streaming_release_failed",
				zap.String("request_id", streamReservationID),
				zap.String("reason", reason),
				zap.Error(err),
			)
		}
		streamReservationPublished = false
		streamReservationAccount = nil
	}
	if streamingBillingV2 {
		streamReservationID = streamingBillingRequestID(c.Request.Context())
		defer func() { releaseStreamReservation("deferred_cleanup") }()
	}

	for {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionHash, reqModel, fs.FailedAccountIDs, "", int64(0))
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error())
				return
			}
			action := fs.HandleSelectionExhausted(c.Request.Context())
			switch action {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				if fs.LastFailoverErr != nil {
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
				} else {
					h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "All available accounts exhausted")
				}
				return
			}
		}
		account := selection.Account

		// 4. Acquire account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
				return
			}
			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				reqStream,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("gateway.cc.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", streamStarted)
				return
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		if streamingBillingV2 && !streamReservationPublished {
			if err := h.executeUsageRecordTask(func(ctx context.Context) error {
				return h.gatewayService.PublishStreamingReserve(ctx, &service.StreamingBillingLifecycleInput{
					RequestID:          streamReservationID,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					Model:              reqModel,
					RequestPayloadHash: requestPayloadHash,
				})
			}); err != nil {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				reqLog.Error("gateway.cc.streaming_reserve_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", streamReservationID),
					zap.Error(err),
				)
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "billing_unavailable", "Billing temporarily unavailable")
				return
			}
			streamReservationPublished = true
			streamReservationAccount = account
			terminalCapture = beginStreamingTerminalCapture(c, true, streamingTerminalModeChatCompletions)
		}

		// 5. Forward request
		responseCapture := beginBufferedResponseCapture(c, queueFirstBilling)
		writerSizeBeforeForward := c.Writer.Size()
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		result, err := h.gatewayService.ForwardAsChatCompletions(c.Request.Context(), c, account, forwardBody, parsedReq)

		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleCCFailoverExhausted(c, failoverErr, true)
					if responseCapture != nil {
						if commitErr := responseCapture.Commit(c); commitErr != nil {
							reqLog.Error("gateway.cc.commit_buffered_response_failed", zap.Error(commitErr))
						}
					}
					return
				}
				action := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
				switch action {
				case FailoverContinue:
					if responseCapture != nil {
						responseCapture.Discard(c)
					}
					// Release any reservation tied to the failing account so
					// the next iteration re-reserves on the replacement.
					releaseStreamReservation("account_switch")
					continue
				case FailoverExhausted:
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
					if responseCapture != nil {
						if commitErr := responseCapture.Commit(c); commitErr != nil {
							reqLog.Error("gateway.cc.commit_buffered_response_failed", zap.Error(commitErr))
						}
					}
					return
				case FailoverCanceled:
					if responseCapture != nil {
						responseCapture.Discard(c)
					}
					return
				}
			}
			h.ensureForwardErrorResponse(c, streamStarted)
			reqLog.Error("gateway.cc.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			if responseCapture != nil {
				if commitErr := responseCapture.Commit(c); commitErr != nil {
					reqLog.Error("gateway.cc.commit_buffered_response_failed", zap.Error(commitErr))
				}
			}
			return
		}

		// 6. Record usage
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		if streamingBillingV2 {
			err = h.executeUsageRecordTask(func(ctx context.Context) error {
				return h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:             result,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					BillingRequestID:   streamReservationID,
					BillingEventKind:   service.UsageChargeEventKindFinalize,
					InboundEndpoint:    inboundEndpoint,
					UpstreamEndpoint:   upstreamEndpoint,
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: requestPayloadHash,
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				})
			})
			if err != nil {
				reqLog.Error("gateway.cc.record_stream_finalize_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", streamReservationID),
					zap.Error(err),
				)
				if terminalCapture != nil {
					_ = terminalCapture.CommitTerminal(c)
				}
				return
			}
			streamReservationFinalized = true
			if terminalCapture != nil {
				if commitErr := terminalCapture.CommitTerminal(c); commitErr != nil {
					reqLog.Error("gateway.cc.commit_terminal_stream_failed", zap.Error(commitErr))
				}
			}
			return
		}

		if queueFirstBilling {
			err = h.executeUsageRecordTask(func(ctx context.Context) error {
				return h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:             result,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					InboundEndpoint:    inboundEndpoint,
					UpstreamEndpoint:   upstreamEndpoint,
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: requestPayloadHash,
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				})
			})
			if err != nil {
				reqLog.Error("gateway.cc.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
				if responseCapture != nil {
					responseCapture.Discard(c)
				}
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "billing_unavailable", "Billing temporarily unavailable")
				return
			}
			if responseCapture != nil {
				if commitErr := responseCapture.Commit(c); commitErr != nil {
					reqLog.Error("gateway.cc.commit_buffered_response_failed", zap.Error(commitErr))
				}
			}
			return
		}
		if reqStream {
			recordLegacyStreamingBilling("/v1/chat/completions")
			reqLog.Debug("gateway.cc.legacy_streaming_billing")
		}
		if responseCapture != nil {
			responseCapture.Discard(c)
		}

		h.submitUsageRecordTask(func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    inboundEndpoint,
				UpstreamEndpoint:   upstreamEndpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
			}); err != nil {
				reqLog.Error("gateway.cc.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
			}
		})
		return
	}
}

// chatCompletionsErrorResponse writes an error in OpenAI Chat Completions format.
func (h *GatewayHandler) chatCompletionsErrorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// handleCCFailoverExhausted writes a failover-exhausted error in CC format.
func (h *GatewayHandler) handleCCFailoverExhausted(c *gin.Context, lastErr *service.UpstreamFailoverError, streamStarted bool) {
	if streamStarted {
		return
	}
	statusCode := http.StatusBadGateway
	if lastErr != nil && lastErr.StatusCode > 0 {
		statusCode = lastErr.StatusCode
	}
	h.chatCompletionsErrorResponse(c, statusCode, "server_error", "All available accounts exhausted")
}
