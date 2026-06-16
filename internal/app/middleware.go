package app

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

type RateLimitConfig struct {
	Enabled       bool
	UseRealIP     bool
	AuthLimit     int
	AuthWindow    time.Duration
	APILimit      int
	APIWindow     time.Duration
	UpdatesLimit  int
	UpdatesWindow time.Duration
}

type SecurityHeadersConfig struct {
	Enabled bool
	HSTS    bool
}

func ProductionRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:       true,
		UseRealIP:     true,
		AuthLimit:     20,
		AuthWindow:    time.Minute,
		APILimit:      120,
		APIWindow:     time.Minute,
		UpdatesLimit:  120,
		UpdatesWindow: time.Minute,
	}
}

func SecurityHeaders(hsts bool) SecurityHeadersConfig {
	return SecurityHeadersConfig{Enabled: true, HSTS: hsts}
}

func (c RateLimitConfig) authMiddleware() func(http.Handler) http.Handler {
	return c.middleware(c.AuthLimit, c.AuthWindow)
}

func (c RateLimitConfig) apiMiddleware() func(http.Handler) http.Handler {
	return c.middleware(c.APILimit, c.APIWindow)
}

func (c RateLimitConfig) updatesMiddleware() func(http.Handler) http.Handler {
	return c.middleware(c.UpdatesLimit, c.UpdatesWindow)
}

func (c RateLimitConfig) middleware(limit int, window time.Duration) func(http.Handler) http.Handler {
	if !c.Enabled || limit <= 0 || window <= 0 {
		return passthrough
	}
	keyFunc := httprate.KeyByIP
	if c.UseRealIP {
		keyFunc = httprate.KeyByRealIP
	}
	return httprate.Limit(limit, window, httprate.WithKeyFuncs(keyFunc, httprate.KeyByEndpoint))
}

func passthrough(next http.Handler) http.Handler {
	return next
}

func securityHeaders(config SecurityHeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !config.Enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			header.Set("X-Frame-Options", "SAMEORIGIN")
			if config.HSTS {
				header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration", time.Since(start).String(),
				"remote", r.RemoteAddr,
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(bytes []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(bytes)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
