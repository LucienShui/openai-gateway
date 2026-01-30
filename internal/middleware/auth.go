package middleware

import (
	"net/http"
	"strings"

	"github.com/LucienShui/openai-gateway/internal/config"
)

func Auth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(cfg.TokenList) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":{"message":"Invalid authorization header","type":"auth_error"}}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if !cfg.ValidateToken(token) {
				http.Error(w, `{"error":{"message":"Invalid API key","type":"auth_error"}}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
