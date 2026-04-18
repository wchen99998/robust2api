package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ChatCompletions handles OpenAI Chat Completions API requests.
// POST /v1/chat/completions
func (h *OpenAIGatewayHandler) ChatCompletions(c *gin.Context) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)
	setOpenAIClientTransportHTTP(c)
	_, span := startOpenAIHandlerSpan(c, "gateway.chat_completions")
	defer span.End()
	tracer := appelotel.GatewayTracer()

	requestStart := time.Now()

	_, authSpan := tracer.Start(c.Request.Context(), "gateway.auth")
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		appelotel.RecordSpanError(authSpan, nil, "invalid api key")
		authSpan.End()
		appelotel.RecordSpanError(span, nil, "invalid api key")
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		appelotel.RecordSpanError(authSpan, nil, "user context not found")
		authSpan.End()
		appelotel.RecordSpanError(span, nil, "user context not found")
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	setOpenAIRequestSpanIdentity(authSpan, apiKey, subject.UserID, "", false)
	authSpan.End()
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.chat_completions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	reqStream := gjson.GetBytes(body, "stream").Bool()
	queueFirstBilling := queueFirstNonStreamEnabled(h.cfg, reqStream)
	streamingBillingV2 := streamingV2Enabled(h.cfg, reqStream)
	requestPayloadHash := service.HashUsageRequestPayload(body)

	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))
	setOpenAIRequestSpanIdentity(span, apiKey, subject.UserID, reqModel, reqStream)

	setOpsRequestContext(c, reqModel, reqStream, body)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	_, billingSpan := tracer.Start(c.Request.Context(), "gateway.billing")
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		appelotel.RecordSpanError(billingSpan, err, err.Error())
		billingSpan.End()
		appelotel.RecordSpanError(span, err, err.Error())
		reqLog.Info("openai_chat_completions.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
	}
	billingSpan.End()

	sessionHash := h.gatewayService.GenerateSessionHash(c, body)
	promptCacheKey := h.gatewayService.ExtractSessionID(c, body)

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError
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
			reqLog.Error("openai_chat_completions.streaming_release_failed",
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
		c.Set("openai_chat_completions_fallback_model", "")
		reqLog.Debug("openai_chat_completions.account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selectCtx, selectSpan := tracer.Start(c.Request.Context(), "gateway.select_account")
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
			selectCtx,
			apiKey.GroupID,
			"",
			sessionHash,
			reqModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportAny,
		)
		if err != nil {
			appelotel.RecordSpanError(selectSpan, err, err.Error())
			selectSpan.End()
			appelotel.RecordSpanError(span, err, err.Error())
			reqLog.Warn("openai_chat_completions.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				defaultModel := ""
				if apiKey.Group != nil {
					defaultModel = apiKey.Group.DefaultMappedModel
				}
				if defaultModel != "" && defaultModel != reqModel {
					reqLog.Info("openai_chat_completions.fallback_to_default_model",
						zap.String("default_mapped_model", defaultModel),
					)
					selection, scheduleDecision, err = h.gatewayService.SelectAccountWithScheduler(
						c.Request.Context(),
						apiKey.GroupID,
						"",
						sessionHash,
						defaultModel,
						failedAccountIDs,
						service.OpenAIUpstreamTransportAny,
					)
					if err == nil && selection != nil {
						c.Set("openai_chat_completions_fallback_model", defaultModel)
					}
				}
				if err != nil {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
					return
				}
			} else {
				if lastFailoverErr != nil {
					h.handleFailoverExhausted(c, lastFailoverErr, streamStarted)
				} else {
					h.handleStreamingAwareError(c, http.StatusBadGateway, "api_error", "Upstream request failed", streamStarted)
				}
				return
			}
		}
		if selection == nil || selection.Account == nil {
			appelotel.RecordSpanError(selectSpan, nil, "no available accounts")
			selectSpan.End()
			appelotel.RecordSpanError(span, nil, "no available accounts")
			h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
			return
		}
		setOpenAIAccountSpanIdentity(selectSpan, selection.Account, "")
		selectSpan.End()
		account := selection.Account
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		reqLog.Debug("openai_chat_completions.account_selected", zap.Int64("account_id", account.ID), zap.String("account_name", account.Name))
		_ = scheduleDecision
		setOpenAIAccountSpanIdentity(span, account, "")

		accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
		if !acquired {
			return
		}

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
				reqLog.Error("openai_chat_completions.streaming_reserve_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", streamReservationID),
					zap.Error(err),
				)
				h.errorResponse(c, http.StatusServiceUnavailable, "billing_unavailable", "Billing temporarily unavailable")
				return
			}
			streamReservationPublished = true
			streamReservationAccount = account
			terminalCapture = beginStreamingTerminalCapture(c, true, streamingTerminalModeChatCompletions)
		}

		responseCapture := beginBufferedResponseCapture(c, queueFirstBilling)
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()

		defaultMappedModel := resolveOpenAIForwardDefaultMappedModel(apiKey, c.GetString("openai_chat_completions_fallback_model"))
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		result, err := h.gatewayService.ForwardAsChatCompletions(c.Request.Context(), c, account, forwardBody, promptCacheKey, defaultMappedModel)

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if err == nil && result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				appelotel.AddSpanEvent(span, appelotel.EventFailoverCandidate,
					appelotel.AttrAccountID(account.ID),
					appelotel.AttrPlatform(account.Platform),
					appelotel.AttrUpstreamStatusCode(failoverErr.StatusCode),
				)
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				// Pool mode: retry on the same account
				if failoverErr.RetryableOnSameAccount {
					retryLimit := account.GetPoolModeRetryCount()
					if sameAccountRetryCount[account.ID] < retryLimit {
						sameAccountRetryCount[account.ID]++
						appelotel.AddSpanEvent(span, appelotel.EventSameAccountRetry,
							appelotel.AttrAccountID(account.ID),
							appelotel.AttrUpstreamStatusCode(failoverErr.StatusCode),
							attribute.Int("sub2api.retry_count", sameAccountRetryCount[account.ID]),
							attribute.Int("sub2api.retry_limit", retryLimit),
						)
						reqLog.Warn("openai_chat_completions.pool_mode_same_account_retry",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
							zap.Int("retry_limit", retryLimit),
							zap.Int("retry_count", sameAccountRetryCount[account.ID]),
						)
						select {
						case <-c.Request.Context().Done():
							if responseCapture != nil {
								responseCapture.Discard(c)
							}
							return
						case <-time.After(sameAccountRetryDelay):
						}
						if responseCapture != nil {
							responseCapture.Discard(c)
						}
						continue
					}
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					appelotel.SetSpanAttributes(span, appelotel.AttrFailoverSwitchCount(switchCount))
					h.handleFailoverExhausted(c, failoverErr, streamStarted)
					if commitErr := commitBufferedResponseOrWriteError(c, responseCapture, func() {
						h.errorResponse(c, http.StatusServiceUnavailable, "server_error", "Response too large")
					}); commitErr != nil {
						reqLog.Error("openai_chat_completions.commit_buffered_response_failed", zap.Error(commitErr))
					}
					return
				}
				switchCount++
				appelotel.SetSpanAttributes(span, appelotel.AttrFailoverSwitchCount(switchCount))
				appelotel.AddSpanEvent(span, appelotel.EventAccountSwitch,
					appelotel.AttrAccountID(account.ID),
					appelotel.AttrUpstreamStatusCode(failoverErr.StatusCode),
					appelotel.AttrFailoverSwitchCount(switchCount),
				)
				reqLog.Warn("openai_chat_completions.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				if responseCapture != nil {
					responseCapture.Discard(c)
				}
				// Release the reservation tied to the failing account so
				// the next iteration re-reserves on the replacement.
				releaseStreamReservation("account_switch")
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			wroteFallback := h.ensureForwardErrorResponse(c, streamStarted)
			if wroteFallback {
				appelotel.AddSpanEvent(span, appelotel.EventFallbackResponse, attribute.Bool("sub2api.stream_started", streamStarted))
			}
			appelotel.RecordSpanError(span, err, err.Error())
			reqLog.Warn("openai_chat_completions.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Bool("fallback_error_response_written", wroteFallback),
				zap.Error(err),
			)
			if commitErr := commitBufferedResponseOrWriteError(c, responseCapture, func() {
				h.errorResponse(c, http.StatusServiceUnavailable, "server_error", "Response too large")
			}); commitErr != nil {
				reqLog.Error("openai_chat_completions.commit_buffered_response_failed", zap.Error(commitErr))
			}
			return
		}
		if result != nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
			setOpenAIAccountSpanIdentity(span, account, result.UpstreamModel)
			appelotel.SetSpanAttributes(span, appelotel.AttrFailoverSwitchCount(switchCount))
			if result.RequestID != "" {
				appelotel.SetSpanAttributes(span, appelotel.AttrUpstreamRequestID(result.RequestID))
			}
		} else {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
		}

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)

		requestSpanCtx := trace.SpanContextFromContext(c.Request.Context())
		if streamingBillingV2 {
			err = h.executeUsageRecordTask(func(ctx context.Context) error {
				usageCtx := ctx
				if requestSpanCtx.IsValid() {
					usageCtx = trace.ContextWithSpanContext(ctx, requestSpanCtx)
				}
				usageCtx, usageSpan := appelotel.GatewayTracer().Start(usageCtx, "gateway.record_usage")
				setOpenAIRequestSpanIdentity(usageSpan, apiKey, subject.UserID, reqModel, reqStream)
				setOpenAIAccountSpanIdentity(usageSpan, account, result.UpstreamModel)
				defer usageSpan.End()
				return h.gatewayService.RecordUsage(usageCtx, &service.OpenAIRecordUsageInput{
					Result:             result,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					BillingRequestID:   streamReservationID,
					BillingEventKind:   service.UsageChargeEventKindFinalize,
					InboundEndpoint:    GetInboundEndpoint(c),
					UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: requestPayloadHash,
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				})
			})
			if err != nil {
				reqLog.Error("openai_chat_completions.record_stream_finalize_failed",
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
					reqLog.Error("openai_chat_completions.commit_terminal_stream_failed", zap.Error(commitErr))
				}
			}
			reqLog.Debug("openai_chat_completions.streaming_request_completed",
				zap.Int64("account_id", account.ID),
				zap.Int("switch_count", switchCount),
			)
			return
		}
		if queueFirstBilling {
			err = h.executeUsageRecordTask(func(ctx context.Context) error {
				usageCtx := ctx
				if requestSpanCtx.IsValid() {
					usageCtx = trace.ContextWithSpanContext(ctx, requestSpanCtx)
				}
				usageCtx, usageSpan := appelotel.GatewayTracer().Start(usageCtx, "gateway.record_usage")
				setOpenAIRequestSpanIdentity(usageSpan, apiKey, subject.UserID, reqModel, reqStream)
				setOpenAIAccountSpanIdentity(usageSpan, account, result.UpstreamModel)
				defer usageSpan.End()
				return h.gatewayService.RecordUsage(usageCtx, &service.OpenAIRecordUsageInput{
					Result:             result,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					InboundEndpoint:    GetInboundEndpoint(c),
					UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: requestPayloadHash,
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				})
			})
			if err != nil {
				reqLog.Error("openai_chat_completions.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
				if responseCapture != nil {
					responseCapture.Discard(c)
				}
				h.errorResponse(c, http.StatusServiceUnavailable, "billing_unavailable", "Billing temporarily unavailable")
				return
			}
			if commitErr := commitBufferedResponseOrWriteError(c, responseCapture, func() {
				h.errorResponse(c, http.StatusServiceUnavailable, "server_error", "Response too large")
			}); commitErr != nil {
				reqLog.Error("openai_chat_completions.commit_buffered_response_failed", zap.Error(commitErr))
			}
			reqLog.Debug("openai_chat_completions.request_completed",
				zap.Int64("account_id", account.ID),
				zap.Int("switch_count", switchCount),
			)
			return
		}
		if responseCapture != nil {
			responseCapture.Discard(c)
		}
		h.submitUsageRecordTask(func(ctx context.Context) {
			usageCtx := ctx
			if requestSpanCtx.IsValid() {
				usageCtx = trace.ContextWithSpanContext(ctx, requestSpanCtx)
			}
			usageCtx, usageSpan := appelotel.GatewayTracer().Start(usageCtx, "gateway.record_usage")
			setOpenAIRequestSpanIdentity(usageSpan, apiKey, subject.UserID, reqModel, reqStream)
			setOpenAIAccountSpanIdentity(usageSpan, account, result.UpstreamModel)
			defer usageSpan.End()
			if err := h.gatewayService.RecordUsage(usageCtx, &service.OpenAIRecordUsageInput{
				Result:             result,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    GetInboundEndpoint(c),
				UpstreamEndpoint:   GetUpstreamEndpoint(c, account.Platform),
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
			}); err != nil {
				appelotel.RecordSpanError(usageSpan, err, err.Error())
				logger.L().With(
					zap.String("component", "handler.openai_gateway.chat_completions"),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", account.ID),
				).Error("openai_chat_completions.record_usage_failed", zap.Error(err))
			}
		})
		reqLog.Debug("openai_chat_completions.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}
