package openai_ws_v2

import (
	"context"
)

// runDuplexRelay 执行双向 WS 帧转发。
// 实现参考了 Caddy reverseproxy 的隧道收敛方式，但这里并不依赖 Caddy 组件或服务。
//
// Reference:
// - Project: caddyserver/caddy (Apache-2.0)
// - Commit: f283062d37c50627d53ca682ebae2ce219b35515
// - Files:
//   - modules/caddyhttp/reverseproxy/streaming.go
//   - modules/caddyhttp/reverseproxy/reverseproxy.go
func runDuplexRelay(
	ctx context.Context,
	clientConn FrameConn,
	upstreamConn FrameConn,
	firstClientMessage []byte,
	options RelayOptions,
) (RelayResult, *RelayExit) {
	return Relay(ctx, clientConn, upstreamConn, firstClientMessage, options)
}
