package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
)

func (s *GatewayService) getUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if s == nil {
		return groupDefaultMultiplier
	}
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(
			s.userGroupRateRepo,
			s.userGroupRateCache,
			resolveUserGroupRateCacheTTL(s.cfg),
			&s.userGroupRateSF,
			"service.gateway",
		)
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefaultMultiplier)
}

// RecordUsageInput 记录使用量的输入参数
type RecordUsageInput struct {
	Result             *ForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription // 可选：订阅信息
	BillingRequestID   string            // 流式 phase-2 使用：预留 request_id，确保 reserve/finalize 使用同一 ID
	BillingEventKind   UsageChargeEventKind
	InboundEndpoint    string             // 入站端点（客户端请求路径）
	UpstreamEndpoint   string             // 上游端点（标准化后的上游路径）
	UserAgent          string             // 请求的 User-Agent
	IPAddress          string             // 请求的客户端 IP 地址
	RequestPayloadHash string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	ForceCacheBilling  bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService      APIKeyQuotaUpdater // 可选：用于更新API Key配额

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// APIKeyQuotaUpdater defines the interface for updating API Key quota and rate limit usage
type APIKeyQuotaUpdater interface {
	UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error
	UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error
}

type apiKeyAuthCacheInvalidator interface {
	InvalidateAuthCacheByKey(ctx context.Context, key string)
}

type usageLogBestEffortWriter interface {
	CreateBestEffort(ctx context.Context, log *UsageLog) error
}

// postUsageBillingParams 统一扣费所需的参数
type postUsageBillingParams struct {
	Cost                  *CostBreakdown
	User                  *User
	APIKey                *APIKey
	Account               *Account
	Subscription          *UserSubscription
	RequestPayloadHash    string
	IsSubscriptionBill    bool
	AccountRateMultiplier float64
	APIKeyService         APIKeyQuotaUpdater
}

func (p *postUsageBillingParams) shouldDeductAPIKeyQuota() bool {
	return p.Cost.ActualCost > 0 && p.APIKey.Quota > 0 && p.APIKeyService != nil
}

func (p *postUsageBillingParams) shouldUpdateRateLimits() bool {
	return p.Cost.ActualCost > 0 && p.APIKey.HasRateLimits() && p.APIKeyService != nil
}

func (p *postUsageBillingParams) shouldUpdateAccountQuota() bool {
	return p.Cost.TotalCost > 0 && p.Account.IsAPIKeyOrBedrock() && p.Account.HasAnyQuotaLimit()
}

func resolveUsageBillingRequestID(ctx context.Context, upstreamRequestID string) string {
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	if requestID := strings.TrimSpace(upstreamRequestID); requestID != "" {
		return requestID
	}
	return "generated:" + generateRequestID()
}

func resolveUsageBillingPayloadFingerprint(ctx context.Context, requestPayloadHash string) string {
	if payloadHash := strings.TrimSpace(requestPayloadHash); payloadHash != "" {
		return payloadHash
	}
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	return ""
}

func buildUsageBillingCommand(requestID string, usageLog *UsageLog, p *postUsageBillingParams) *UsageBillingCommand {
	if p == nil || p.Cost == nil || p.APIKey == nil || p.User == nil || p.Account == nil {
		return nil
	}

	cmd := &UsageBillingCommand{
		RequestID:          requestID,
		APIKeyID:           p.APIKey.ID,
		UserID:             p.User.ID,
		AccountID:          p.Account.ID,
		AccountType:        p.Account.Type,
		RequestPayloadHash: strings.TrimSpace(p.RequestPayloadHash),
	}
	if usageLog != nil {
		cmd.Model = usageLog.Model
		cmd.BillingType = usageLog.BillingType
		cmd.InputTokens = usageLog.InputTokens
		cmd.OutputTokens = usageLog.OutputTokens
		cmd.CacheCreationTokens = usageLog.CacheCreationTokens
		cmd.CacheReadTokens = usageLog.CacheReadTokens
		cmd.ImageCount = usageLog.ImageCount
		if usageLog.ServiceTier != nil {
			cmd.ServiceTier = *usageLog.ServiceTier
		}
		if usageLog.ReasoningEffort != nil {
			cmd.ReasoningEffort = *usageLog.ReasoningEffort
		}
		if usageLog.SubscriptionID != nil {
			cmd.SubscriptionID = usageLog.SubscriptionID
		}
	}

	if p.IsSubscriptionBill && p.Subscription != nil && p.Cost.TotalCost > 0 {
		cmd.SubscriptionID = &p.Subscription.ID
		cmd.SubscriptionCost = p.Cost.TotalCost
	} else if p.Cost.ActualCost > 0 {
		cmd.BalanceCost = p.Cost.ActualCost
	}

	if p.shouldDeductAPIKeyQuota() {
		cmd.APIKeyQuotaCost = p.Cost.ActualCost
	}
	if p.shouldUpdateRateLimits() {
		cmd.APIKeyRateLimitCost = p.Cost.ActualCost
	}
	if p.shouldUpdateAccountQuota() {
		cmd.AccountQuotaCost = p.Cost.TotalCost * p.AccountRateMultiplier
	}

	cmd.Normalize()
	return cmd
}

func detachedBillingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, postUsageBillingTimeout)
}

func detachStreamUpstreamContext(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	if !stream {
		return ctx, func() {}
	}
	if ctx == nil {
		return context.Background(), func() {}
	}
	return context.WithoutCancel(ctx), func() {}
}

func writeUsageLogBestEffort(ctx context.Context, repo UsageLogRepository, usageLog *UsageLog, logKey string) {
	if repo == nil || usageLog == nil {
		return
	}
	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	if writer, ok := repo.(usageLogBestEffortWriter); ok {
		if err := writer.CreateBestEffort(usageCtx, usageLog); err != nil {
			logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
			if IsUsageLogCreateDropped(err) {
				return
			}
			if _, syncErr := repo.Create(usageCtx, usageLog); syncErr != nil {
				logger.LegacyPrintf(logKey, "Create usage log sync fallback failed: %v", syncErr)
			}
		}
		return
	}

	if _, err := repo.Create(usageCtx, usageLog); err != nil {
		logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
	}
}

// recordUsageOpts 内部选项，参数化 RecordUsage 与 RecordUsageWithLongContext 的差异点。
type recordUsageOpts struct {
	// Claude Max 策略所需的 ParsedRequest（可选，仅 Claude 路径传入）
	ParsedRequest *ParsedRequest

	// EnableClaudePath 启用 Claude 路径特有逻辑：
	// - Claude Max 缓存计费策略
	EnableClaudePath bool

	// 长上下文计费（仅 Gemini 路径需要）
	LongContextThreshold  int
	LongContextMultiplier float64
}

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		BillingRequestID:   input.BillingRequestID,
		BillingEventKind:   input.BillingEventKind,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{
		EnableClaudePath: true,
	})
}

// RecordUsageLongContextInput 记录使用量的输入参数（支持长上下文双倍计费）
type RecordUsageLongContextInput struct {
	Result                *ForwardResult
	APIKey                *APIKey
	User                  *User
	Account               *Account
	Subscription          *UserSubscription // 可选：订阅信息
	BillingRequestID      string            // 流式 phase-2 使用：预留 request_id，确保 reserve/finalize 使用同一 ID
	BillingEventKind      UsageChargeEventKind
	InboundEndpoint       string             // 入站端点（客户端请求路径）
	UpstreamEndpoint      string             // 上游端点（标准化后的上游路径）
	UserAgent             string             // 请求的 User-Agent
	IPAddress             string             // 请求的客户端 IP 地址
	RequestPayloadHash    string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	LongContextThreshold  int                // 长上下文阈值（如 200000）
	LongContextMultiplier float64            // 超出阈值部分的倍率（如 2.0）
	ForceCacheBilling     bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService         APIKeyQuotaUpdater // API Key 配额服务（可选）

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// RecordUsageWithLongContext 记录使用量并扣费，支持长上下文双倍计费（用于 Gemini）
func (s *GatewayService) RecordUsageWithLongContext(ctx context.Context, input *RecordUsageLongContextInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		BillingRequestID:   input.BillingRequestID,
		BillingEventKind:   input.BillingEventKind,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{
		LongContextThreshold:  input.LongContextThreshold,
		LongContextMultiplier: input.LongContextMultiplier,
	})
}

// recordUsageCoreInput 是 recordUsageCore 的公共输入字段，从两种输入结构体中提取。
type recordUsageCoreInput struct {
	Result             *ForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	BillingRequestID   string
	BillingEventKind   UsageChargeEventKind
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	ForceCacheBilling  bool
	APIKeyService      APIKeyQuotaUpdater
	ChannelUsageFields
}

// recordUsageCore 是 RecordUsage 和 RecordUsageWithLongContext 的统一实现。
// opts 中的字段控制两者之间的差异行为：
// - ParsedRequest != nil → 启用 Claude Max 缓存计费策略
// - LongContextThreshold > 0 → Token 计费回退走 CalculateCostWithLongContext
func (s *GatewayService) recordUsageCore(ctx context.Context, input *recordUsageCoreInput, opts *recordUsageOpts) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 强制缓存计费：将 input_tokens 转为 cache_read_input_tokens
	// 用于粘性会话切换时的特殊计费处理
	if input.ForceCacheBilling && result.Usage.InputTokens > 0 {
		logger.LegacyPrintf("service.gateway", "force_cache_billing: %d input_tokens → cache_read_input_tokens (account=%d)",
			result.Usage.InputTokens, account.ID)
		result.Usage.CacheReadInputTokens += result.Usage.InputTokens
		result.Usage.InputTokens = 0
	}

	// Cache TTL Override: 确保计费时 token 分类与账号设置一致
	cacheTTLOverridden := false
	if account.IsCacheTTLOverrideEnabled() {
		applyCacheTTLOverride(&result.Usage, account.GetCacheTTLOverrideTarget())
		cacheTTLOverridden = (result.Usage.CacheCreation5mTokens + result.Usage.CacheCreation1hTokens) > 0
	}

	// Record token metrics via OTel business metrics.
	{
		platform := account.Platform
		model := result.Model
		if result.FirstTokenMs != nil && *result.FirstTokenMs > 0 {
			appelotel.M().RecordTTFT(ctx, float64(*result.FirstTokenMs)/1000, platform, model)
		}
		if result.Usage.InputTokens > 0 {
			appelotel.M().RecordTokens(ctx, int64(result.Usage.InputTokens), "input", platform, model)
		}
		if result.Usage.OutputTokens > 0 {
			appelotel.M().RecordTokens(ctx, int64(result.Usage.OutputTokens), "output", platform, model)
		}
		if result.Usage.CacheCreationInputTokens > 0 {
			appelotel.M().RecordTokens(ctx, int64(result.Usage.CacheCreationInputTokens), "cache_creation", platform, model)
		}
		if result.Usage.CacheReadInputTokens > 0 {
			appelotel.M().RecordTokens(ctx, int64(result.Usage.CacheReadInputTokens), "cache_read", platform, model)
		}
	}

	// 获取费率倍数（优先级：用户专属 > 分组默认 > 系统默认）
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		groupDefault := apiKey.Group.RateMultiplier
		multiplier = s.getUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, groupDefault)
	}

	// 确定计费模型
	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}

	// 确定 RequestedModel（渠道映射前的原始模型）
	requestedModel := result.Model
	if input.OriginalModel != "" {
		requestedModel = input.OriginalModel
	}

	// 计算费用
	cost := s.calculateRecordUsageCost(ctx, result, apiKey, billingModel, multiplier, opts)

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// 创建使用日志
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := s.buildRecordUsageLog(ctx, input, result, apiKey, user, account, subscription,
		requestedModel, multiplier, accountRateMultiplier, billingType, cacheTTLOverridden, cost, opts)

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")
		logger.LegacyPrintf("service.gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
	}

	// Build the billing command and publish to the billing stream.
	// When a caller-supplied BillingRequestID is present (streaming v2
	// finalize), use it for both the event and the persisted usage log so
	// reserve/finalize remain correlated and the ledger/usage_log rows agree.
	requestID := strings.TrimSpace(input.BillingRequestID)
	if requestID == "" {
		requestID = usageLog.RequestID
	} else {
		usageLog.RequestID = requestID
	}
	p := &postUsageBillingParams{
		Cost:                  cost,
		User:                  user,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         input.APIKeyService,
	}
	cmd := buildUsageBillingCommand(requestID, usageLog, p)
	if cmd == nil {
		// Non-simple mode usage persistence is authoritative in the billing consumer.
		return nil
	}

	var groupID int64
	if apiKey.GroupID != nil {
		groupID = *apiKey.GroupID
	}
	eventKind := input.BillingEventKind
	if strings.TrimSpace(string(eventKind)) == "" {
		eventKind = UsageChargeEventKindCharge
	}
	event := NewUsageChargeEventWithKind(eventKind, cmd, usageLog, groupID, apiKey.HasRateLimits())
	if s.billingPublisher == nil {
		slog.Error("billing event publisher unavailable",
			"component", "service.gateway",
			"request_id", requestID,
			"user_id", user.ID,
		)
		return ErrBillingEventPublisherUnavailable
	}
	if err := s.billingPublisher.Publish(ctx, event); err != nil {
		slog.Error("billing event publish failed",
			"component", "service.gateway",
			"request_id", requestID,
			"user_id", user.ID,
			"error", err,
		)
		return err
	}

	return nil
}

// calculateRecordUsageCost 根据请求类型和选项计算费用。
func (s *GatewayService) calculateRecordUsageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	// 图片生成计费
	if result.ImageCount > 0 {
		return s.calculateImageCost(ctx, result, apiKey, billingModel, multiplier)
	}

	// Token 计费
	return s.calculateTokenCost(ctx, result, apiKey, billingModel, multiplier, opts)
}

// resolveChannelPricing 检查指定模型是否存在渠道级别定价。
// 返回非 nil 的 ResolvedPricing 表示有渠道定价，nil 表示走默认定价路径。
func (s *GatewayService) resolveChannelPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s.resolver == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &gid})
	if resolved.Source == PricingSourceChannel {
		return resolved
	}
	return nil
}

// calculateImageCost 计算图片生成费用：渠道级别定价优先，否则走按次计费。
func (s *GatewayService) calculateImageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
) *CostBreakdown {
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		tokens := UsageTokens{
			InputTokens:       result.Usage.InputTokens,
			OutputTokens:      result.Usage.OutputTokens,
			ImageOutputTokens: result.Usage.ImageOutputTokens,
		}
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Calculate image token cost failed: %v", err)
			return &CostBreakdown{ActualCost: 0}
		}
		return cost
	}

	var groupConfig *ImagePriceConfig
	if apiKey.Group != nil {
		groupConfig = &ImagePriceConfig{
			Price1K: apiKey.Group.ImagePrice1K,
			Price2K: apiKey.Group.ImagePrice2K,
			Price4K: apiKey.Group.ImagePrice4K,
		}
	}
	return s.billingService.CalculateImageCost(billingModel, result.ImageSize, result.ImageCount, groupConfig, multiplier)
}

// calculateTokenCost 计算 Token 计费：根据 opts 决定走普通/长上下文/渠道统一计费。
func (s *GatewayService) calculateTokenCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	opts *recordUsageOpts,
) *CostBreakdown {
	tokens := UsageTokens{
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
	}

	var cost *CostBreakdown
	var err error

	// 优先尝试渠道定价 → CalculateCostUnified
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		gid := apiKey.Group.ID
		cost, err = s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
	} else if opts.LongContextThreshold > 0 {
		// 长上下文双倍计费（如 Gemini 200K 阈值）
		cost, err = s.billingService.CalculateCostWithLongContext(
			billingModel, tokens, multiplier,
			opts.LongContextThreshold, opts.LongContextMultiplier,
		)
	} else {
		cost, err = s.billingService.CalculateCost(billingModel, tokens, multiplier)
	}
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Calculate cost failed: %v", err)
		return &CostBreakdown{ActualCost: 0}
	}
	return cost
}

// buildRecordUsageLog 构建使用日志并设置计费模式。
func (s *GatewayService) buildRecordUsageLog(
	ctx context.Context,
	input *recordUsageCoreInput,
	result *ForwardResult,
	apiKey *APIKey,
	user *User,
	account *Account,
	subscription *UserSubscription,
	requestedModel string,
	multiplier float64,
	accountRateMultiplier float64,
	billingType int8,
	cacheTTLOverridden bool,
	cost *CostBreakdown,
	opts *recordUsageOpts,
) *UsageLog {
	durationMs := int(result.Duration.Milliseconds())
	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 result.Model,
		RequestedModel:        requestedModel,
		UpstreamModel:         optionalNonEqualStringPtr(result.UpstreamModel, result.Model),
		ReasoningEffort:       result.ReasoningEffort,
		InboundEndpoint:       optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:      optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		RequestType:           RequestTypeFromLegacy(result.Stream, false),
		BillingMode:           resolveBillingMode(result, cost),
		Stream:                result.Stream,
		DurationMs:            &durationMs,
		FirstTokenMs:          result.FirstTokenMs,
		ImageCount:            result.ImageCount,
		ImageSize:             optionalTrimmedStringPtr(result.ImageSize),
		CacheTTLOverridden:    cacheTTLOverridden,
		ChannelID:             optionalInt64Ptr(input.ChannelID),
		ModelMappingChain:     optionalTrimmedStringPtr(input.ModelMappingChain),
		UserAgent:             optionalTrimmedStringPtr(input.UserAgent),
		IPAddress:             optionalTrimmedStringPtr(input.IPAddress),
		GroupID:               apiKey.GroupID,
		SubscriptionID:        optionalSubscriptionID(subscription),
		CreatedAt:             time.Now(),
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
	}

	return usageLog
}

// resolveBillingMode 根据计费结果和请求类型确定计费模式。
func resolveBillingMode(result *ForwardResult, cost *CostBreakdown) *string {
	var mode string
	switch {
	case cost != nil && cost.BillingMode != "":
		mode = cost.BillingMode
	case result.ImageCount > 0:
		mode = string(BillingModeImage)
	default:
		mode = string(BillingModeToken)
	}
	return &mode
}

func optionalSubscriptionID(subscription *UserSubscription) *int64 {
	if subscription != nil {
		return &subscription.ID
	}
	return nil
}
