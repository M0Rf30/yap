// Package mcp — logging middleware.
//
// The MCP stdio transport reserves stdout for JSON-RPC frames; anything
// written there corrupts the protocol stream. pkg/logger writes to stderr by
// default, so it is safe to use from server middleware.
//
// loggingMiddleware logs every inbound (server-receiving) and outbound
// (server-sending) MCP method call with method name, duration, and error
// status. Tool arguments and results are intentionally not logged — they may
// contain large blobs or sensitive paths.
package mcp

import (
	"context"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// loggingMiddleware returns an MCP Middleware that records each method call.
// direction is "recv" for AddReceivingMiddleware and "send" for
// AddSendingMiddleware; it is surfaced as a structured log field.
func loggingMiddleware(direction string) mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request,
		) (mcpsdk.Result, error) {
			start := time.Now()

			logger.Debug("mcp request",
				"direction", direction,
				"method", method,
			)

			res, err := next(ctx, method, req)

			elapsed := time.Since(start)
			if err != nil {
				logger.Error("mcp request failed",
					"direction", direction,
					"method", method,
					"duration", elapsed,
					"error", err,
				)

				return res, err
			}

			logger.Info("mcp request done",
				"direction", direction,
				"method", method,
				"duration", elapsed,
			)

			return res, nil
		}
	}
}
