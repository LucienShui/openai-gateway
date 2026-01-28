package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LucienShui/openai-gateway/internal/config"
	"github.com/LucienShui/openai-gateway/internal/handler"
	"github.com/LucienShui/openai-gateway/internal/logger"
	"github.com/LucienShui/openai-gateway/internal/middleware"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type ProxyRequest struct {
	URL  string          `json:"url"`
	Body json.RawMessage `json:"body"`
}

type TextRequest struct {
	Text string `json:"text"`
}

func main() {
	configJSON := os.Getenv("CONFIG")
	apiKeys := os.Getenv("API_KEYS")

	cfg, err := config.NewConfig(configJSON, apiKeys)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	lgr := logger.New(os.Stdout)
	h := handler.New(cfg, lgr)

	r := chi.NewRouter()

	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(corsMiddleware)

	r.Get("/health", h.Health)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(cfg))
		r.Get("/v1/models", h.Models)
		r.Post("/v1/completions", h.Proxy)
		r.Post("/v1/chat/completions", h.Proxy)
		r.Post("/v1/embeddings", h.Proxy)
		r.Post("/v1/responses", h.Proxy)
	})

	port := config.GetEnv("PORT", "8000")
	host := config.GetEnv("HOST", "0.0.0.0")
	addr := host + ":" + port

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("Starting server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	var req ProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", req.URL, bytes.NewReader(req.Body))
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers from client request to upstream request
	for key, values := range r.Header {
		// Skip hop-by-hop headers
		if key == "Content-Length" || key == "Connection" || key == "Host" {
			continue
		}
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
		}
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")

	// Make upstream request
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		http.Error(w, "failed to connect to upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Set SSE headers for client response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Stream response from upstream to client
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("error reading upstream: %v", err)
			}
			break
		}
		w.Write(line)
		flusher.Flush()
	}
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	var req TextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for _, char := range req.Text {
		fmt.Fprintf(w, "data: %s\n\n", string(char))
		flusher.Flush()
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
