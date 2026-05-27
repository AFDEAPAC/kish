package middleware

import (
	"net/http"
	"strings"

	"github.com/AFDEAPAC/kish/internal/config"
)

// CORS returns middleware that handles browser cross-origin requests when
// a deployment serves the dashboard from a different origin than the API.
// Same-origin reverse-proxy deployments should leave CORS disabled in
// config (`cors.enabled = false`) so no cross-origin headers leak.
//
// cfg comes from the cors.* section of the application config. The middleware
// reads AllowedOrigins, AllowedMethods, AllowedHeaders, and AllowCredentials
// once at construction; runtime config reload is not supported.
//
// Security note: when AllowCredentials is true the wildcard "*" origin is
// intentionally not honoured because the browser will reject the combination
// anyway and silently dropping the header is safer than returning credentials
// to an unknown origin (see originAllowed). Operators that need both must
// list every concrete origin explicitly.
func CORS(cfg config.CORSConfig) func(http.Handler) http.Handler {
	allowedOrigins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowedOrigins[trimmed] = struct{}{}
		}
	}
	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		if !cfg.Enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !originAllowed(origin, allowedOrigins, cfg.AllowCredentials) {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Origin", origin)
			if cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if allowedMethods != "" {
				h.Set("Access-Control-Allow-Methods", allowedMethods)
			}
			if allowedHeaders != "" {
				h.Set("Access-Control-Allow-Headers", allowedHeaders)
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed map[string]struct{}, allowCredentials bool) bool {
	if _, ok := allowed[origin]; ok {
		return true
	}
	if _, ok := allowed["*"]; ok {
		return !allowCredentials
	}
	return false
}
