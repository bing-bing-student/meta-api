package middlewares

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const redactedLogValue = "[REDACTED]"

func dumpSanitizedRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	clonedRequest := r.Clone(r.Context())
	clonedRequest.Header = r.Header.Clone()
	redactSensitiveHeaders(clonedRequest.Header)

	if r.URL != nil {
		sanitizedURL := *r.URL
		sanitizedURL.RawQuery = sanitizeQueryValues(r.URL.Query()).Encode()
		clonedRequest.URL = &sanitizedURL
		clonedRequest.RequestURI = sanitizedURL.RequestURI()
	}

	httpRequest, err := httputil.DumpRequest(clonedRequest, false)
	if err != nil {
		return ""
	}
	return string(httpRequest)
}

func redactSensitiveHeaders(headers http.Header) {
	for name := range headers {
		if isSensitiveLogField(name) {
			headers.Set(name, redactedLogValue)
		}
	}
}

func sanitizeQueryValues(values url.Values) url.Values {
	sanitized := make(url.Values, len(values))
	for name, rawValues := range values {
		copiedValues := append([]string(nil), rawValues...)
		if isSensitiveLogField(name) {
			copiedValues = []string{redactedLogValue}
		}
		sanitized[name] = copiedValues
	}
	return sanitized
}

func isSensitiveLogField(name string) bool {
	normalizedName := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	if normalizedName == "code" ||
		normalizedName == "otp" ||
		normalizedName == "totp" ||
		normalizedName == "password" ||
		normalizedName == "authorization" ||
		normalizedName == "cookie" ||
		normalizedName == "set-cookie" ||
		normalizedName == "loginchallenge" ||
		normalizedName == "login-challenge" {
		return true
	}

	return strings.Contains(normalizedName, "token") ||
		strings.Contains(normalizedName, "secret") ||
		strings.Contains(normalizedName, "password") ||
		strings.Contains(normalizedName, "code") ||
		strings.Contains(normalizedName, "credential")
}

// GinRecovery recover掉项目可能出现的panic
func GinRecovery(logger *zap.Logger, stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				if e, ok := err.(error); ok {
					var netErr net.Error
					if errors.As(e, &netErr) {
						if strings.Contains(strings.ToLower(e.Error()), "broken pipe") ||
							strings.Contains(strings.ToLower(e.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest := dumpSanitizedRequest(c.Request)
				if brokenPipe {
					logger.Error(c.Request.URL.Path, zap.Any("code", err), zap.String("request", httpRequest))
					_ = c.Error(err.(error))
					c.Abort()
					return
				}

				if stack {
					logger.Error("[Recovery from panic]",
						zap.Any("code", err),
						zap.String("request", httpRequest),
						zap.String("stack", string(debug.Stack())),
					)
				} else {
					logger.Error("[Recovery from panic]",
						zap.Any("code", err),
						zap.String("request", httpRequest),
					)
				}
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
