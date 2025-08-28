// Package huma provides reusable logic related to the huma framework.
package huma

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
)

// NewLoggerMiddleware is a middleware that logs the request.
// It logs the request method and path.
func NewLoggerMiddleware(logger *slog.Logger) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		logger.Info("Request received", "method", ctx.Method(), "path", ctx.URL().Path)
		next(ctx)
	}
}

var reqid uint64

// NewRequestIDMiddleware is a middleware that injects a request ID into the context of each
// request. A request ID is a string of the form "host.example.com/random-0001",
// where "random" is a base62 random string that uniquely identifies this go
// process, and where the last number is an atomically incremented request
// counter.
func NewRequestIDMiddleware(prefix string) func(ctx huma.Context, next func(huma.Context)) {

	return func(ctx huma.Context, next func(huma.Context)) {
		requestID := ctx.Header("X-Request-ID")
		if requestID == "" {
			myid := atomic.AddUint64(&reqid, 1)
			requestID = fmt.Sprintf("%s-%06d", prefix, myid)
		}
		ctx.SetHeader("X-Request-ID", requestID)
		next(ctx)
	}
}

var trueClientIP = http.CanonicalHeaderKey("True-Client-IP")
var xForwardedFor = http.CanonicalHeaderKey("X-Forwarded-For")
var xRealIP = http.CanonicalHeaderKey("X-Real-IP")

// GetRealIPMiddleware is a middleware that sets a http.Request's RemoteAddr to the results
// of parsing either the True-Client-IP, X-Real-IP or the X-Forwarded-For headers
// (in that order).
//
// This middleware should be inserted fairly early in the middleware stack to
// ensure that subsequent layers (e.g., request loggers) which examine the
// RemoteAddr will see the intended value.
//
// You should only use this middleware if you can trust the headers passed to
// you (in particular, the three headers this middleware uses), for example
// because you have placed a reverse proxy like HAProxy or nginx in front of
// chi. If your reverse proxies are configured to pass along arbitrary header
// values from the client, or if you use this middleware without a reverse
// proxy, malicious clients will be able to make you very sad (or, depending on
// how you're using RemoteAddr, vulnerable to an attack of some sort).
func GetRealIPMiddleware(ctx huma.Context, next func(huma.Context)) {
	if rip := realIP(ctx); rip != "" {
		ctx.SetHeader("X-Real-IP", rip)
	}
	next(ctx)
}

func realIP(c huma.Context) string {
	var ip string

	if tcip := c.Header(trueClientIP); tcip != "" {
		ip = tcip
	} else if xrip := c.Header(xRealIP); xrip != "" {
		ip = xrip
	} else if xff := c.Header(xForwardedFor); xff != "" {
		ip, _, _ = strings.Cut(xff, ",")
	}
	if ip == "" || net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}
