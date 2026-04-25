package domain

type Platform string

const (
	PlatformOpenAI      Platform = "openai"
	PlatformAnthropic   Platform = "anthropic"
	PlatformGemini      Platform = "gemini"
	PlatformAntigravity Platform = "antigravity"
)

type EndpointKind string

const (
	EndpointUnknown               EndpointKind = "unknown"
	EndpointOpenAIResponses       EndpointKind = "openai_responses"
	EndpointOpenAIChatCompletions EndpointKind = "openai_chat_completions"
	EndpointOpenAIMessages        EndpointKind = "openai_messages"
	EndpointAnthropicMessages     EndpointKind = "anthropic_messages"
	EndpointAnthropicCountTokens  EndpointKind = "anthropic_count_tokens"
	EndpointGeminiNative          EndpointKind = "gemini_native"
	EndpointAntigravity           EndpointKind = "antigravity"
)

type AccountType string

const (
	AccountTypeUnknown AccountType = "unknown"
	AccountTypeAPIKey  AccountType = "api_key"
	AccountTypeOAuth   AccountType = "oauth"
)

type TransportKind string

const (
	TransportUnknown   TransportKind = "unknown"
	TransportHTTP      TransportKind = "http"
	TransportSSE       TransportKind = "sse"
	TransportWebSocket TransportKind = "websocket"
)
