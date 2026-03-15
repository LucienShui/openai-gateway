package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/LucienShui/openai-gateway/internal/config"
	"github.com/LucienShui/openai-gateway/internal/handler"
	"github.com/LucienShui/openai-gateway/internal/logger"
	"github.com/LucienShui/openai-gateway/internal/middleware"
	"github.com/LucienShui/openai-gateway/internal/telemetry"
)

func main() {
	ctx := context.Background()

	shutdownTelemetry, err := telemetry.Init(ctx)
	if err != nil {
		log.Fatalf("failed to initialize telemetry: %v", err)
	}

	configJSON := os.Getenv("CONFIG")
	apiKeys := os.Getenv("API_KEYS")

	cfg, err := config.NewConfig(configJSON, apiKeys)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	appLogger := logger.New()
	defer func() { _ = appLogger.Sync() }()

	// Create API logger that writes to both console and file
	apiLogger := logger.NewAPILogger(appLogger)
	defer func() { _ = apiLogger.Sync() }()

	h := handler.New(cfg, apiLogger)

	r := chi.NewRouter()

	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(corsMiddleware)

	r.NotFound(h.NotFound)

	r.Get("/health", h.Health)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(cfg))
		r.Get("/v1/models", h.Models)
		r.Post("/v1/chat/completions", h.Proxy)
		r.Post("/v1/completions", h.Proxy)
		r.Post("/v1/embeddings", h.Proxy)
		r.Post("/v1/responses", h.Proxy)
	})

	port := config.GetEnv("PORT", "8000")
	host := config.GetEnv("HOST", "0.0.0.0")
	addr := host + ":" + port

	srv := &http.Server{
		Addr: addr,
		Handler: otelhttp.NewHandler(r, "openai-gateway", otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/health"
		})),
	}

	go func() {
		appLogger.Info("Starting server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatal("listen", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	appLogger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := shutdownTelemetry(ctx); err != nil {
		appLogger.Error("failed to shutdown telemetry", zap.Error(err))
	}

	if err := srv.Shutdown(ctx); err != nil {
		appLogger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	appLogger.Info("Server exited")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
