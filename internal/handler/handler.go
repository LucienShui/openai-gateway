package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/LucienShui/openai-gateway/internal/config"
	"github.com/LucienShui/openai-gateway/internal/logger"
	"github.com/LucienShui/openai-gateway/internal/telemetry"
)

type Handler struct {
	cfg       *config.Config
	logger    *zap.Logger
	startTime time.Time
}

func New(cfg *config.Config, log *zap.Logger) *Handler {
	return &Handler{cfg: cfg, logger: log, startTime: time.Now()}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"uptime": time.Since(h.startTime).String(),
	})
}

func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.errorResponse(w, http.StatusNotFound, fmt.Sprintf("path not found: %s", r.URL.Path))
}

func (h *Handler) Models(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.cfg.ModelList)
}

func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := r.Header.Get("X-Request-Id")
	path := r.URL.Path

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	err = r.Body.Close()
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "failed to close request body")
		return
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	modelName, ok := reqBody["model"].(string)
	if !ok {
		h.errorResponse(w, http.StatusBadRequest, "model field is required")
		return
	}

	route, ok := h.cfg.GetRoute(modelName)
	if !ok {
		h.errorResponse(w, http.StatusNotFound, fmt.Sprintf("model not found: %s", modelName))
		return
	}

	reqBody["model"] = route.Model
	modifiedBody, _ := json.Marshal(reqBody)

	upstreamURL := route.Client.BuildURL(path)

	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(modifiedBody))
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "failed to create upstream request")
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	if route.Client.IsAzure {
		upstreamReq.Header.Set("api-key", route.Client.APIKey)
	} else {
		upstreamReq.Header.Set("Authorization", "Bearer "+route.Client.APIKey)
	}

	isStream, _ := reqBody["stream"].(bool)

	resp, err := route.Client.HTTPClient.Do(upstreamReq)
	if err != nil {
		h.logger.Error("upstream request failed",
			zap.String("api", path),
			zap.String("error", err.Error()),
			zap.String("request_id", requestID),
		)
		h.errorResponse(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, v := range resp.Header {
		if k == "Content-Length" || k == "Transfer-Encoding" {
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	if isStream {
		h.handleStream(r.Context(), w, resp, path, reqBody, requestID, startTime)
	} else {
		h.handleNonStream(r.Context(), w, resp, path, reqBody, requestID, startTime)
	}
}

func (h *Handler) handleStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, path string, reqBody map[string]any, requestID string, startTime time.Time) {
	ctx, span := telemetry.StartSpan(ctx, "stream")
	defer span.End()

	// If upstream returned an error, don't stream - return JSON response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.handleNonStream(ctx, w, resp, path, reqBody, requestID, startTime)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("streaming not supported",
			zap.String("api", path),
			zap.String("request_id", requestID),
		)
		return
	}

	var responseContent strings.Builder
	var reasoningContent strings.Builder
	var lastChunk string
	var streamError string

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		_, _ = fmt.Fprintf(w, "%s\n\n", line)
		flusher.Flush()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}
			lastChunk = data

			// Check for error in stream chunk
			if h.hasError(data) {
				streamError = data
				telemetry.SetSpanError(span, fmt.Sprintf("upstream stream error: %s", data))
				h.logger.Error("upstream stream error",
					zap.String("api", path),
					zap.String("error_chunk", data),
					zap.String("request_id", requestID),
				)
			} else {
				h.extractContent(data, &responseContent, &reasoningContent)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		telemetry.RecordError(span, err, "stream scan error")
		h.logger.Error("stream scan error",
			zap.String("api", path),
			zap.Error(err),
			zap.String("request_id", requestID),
		)
	}

	if logger.IsDebug() {
		reqJSON, _ := json.Marshal(excludeEmbedding(reqBody))
		fields := []zap.Field{
			zap.String("api", path),
			zap.String("request", string(reqJSON)),
			zap.String("response", responseContent.String()),
			zap.Float64("time", time.Since(startTime).Seconds()),
		}
		if requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
		if reasoningContent.Len() > 0 {
			fields = append(fields, zap.String("reasoning_content", reasoningContent.String()))
		}
		if lastChunk != "" {
			fields = append(fields, zap.String("chunk", lastChunk))
		}
		if streamError != "" {
			fields = append(fields, zap.String("stream_error", streamError))
		}
		h.logger.Debug("stream completed", fields...)
	}

	// Set span attributes
	reqJSON, _ := json.Marshal(excludeEmbedding(reqBody))
	telemetry.SetSpanAttributes(span,
		attribute.String("api", path),
		attribute.String("request", string(reqJSON)),
		attribute.String("response", responseContent.String()),
	)
	if requestID != "" {
		telemetry.SetSpanAttributes(span, attribute.String("request_id", requestID))
	}
	if reasoningContent.Len() > 0 {
		telemetry.SetSpanAttributes(span, attribute.String("reasoning_content", reasoningContent.String()))
	}
	if lastChunk != "" {
		telemetry.SetSpanAttributes(span, attribute.String("chunk", lastChunk))
	}
	if streamError != "" {
		telemetry.SetSpanAttributes(span, attribute.String("stream_error", streamError))
	}
}

func (h *Handler) handleNonStream(ctx context.Context, w http.ResponseWriter, resp *http.Response, path string, reqBody map[string]any, requestID string, startTime time.Time) {
	_, span := telemetry.StartSpan(ctx, path)
	defer span.End()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		telemetry.RecordError(span, err, "failed to read upstream response")
		h.errorResponse(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		telemetry.SetSpanError(span, fmt.Sprintf("upstream error: %d", resp.StatusCode))
		telemetry.SetSpanAttributes(span, attribute.String("error_response", string(respBody)))
		h.logger.Warn("upstream returned error",
			zap.String("api", path),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(respBody)),
			zap.String("request_id", requestID),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)

	var respJSON map[string]any
	_ = json.Unmarshal(respBody, &respJSON)

	if logger.IsDebug() {
		reqJSON, _ := json.Marshal(excludeEmbedding(reqBody))
		respJSONBytes, _ := json.Marshal(excludeEmbedding(respJSON))
		fields := []zap.Field{
			zap.String("api", path),
			zap.String("request", string(reqJSON)),
			zap.String("response", string(respJSONBytes)),
			zap.Float64("time", time.Since(startTime).Seconds()),
		}
		if requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
		h.logger.Debug("request completed", fields...)
	}

	// Set span attributes
	reqJSON, _ := json.Marshal(excludeEmbedding(reqBody))
	respJSONBytes, _ := json.Marshal(excludeEmbedding(respJSON))
	telemetry.SetSpanAttributes(span,
		attribute.String("api", path),
		attribute.String("request", string(reqJSON)),
		attribute.String("response", string(respJSONBytes)),
	)
	if requestID != "" {
		telemetry.SetSpanAttributes(span, attribute.String("request_id", requestID))
	}
}

func (h *Handler) hasError(data string) bool {
	var chunk map[string]any
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false
	}

	_, ok := chunk["error"].(map[string]any)
	return ok
}

func (h *Handler) extractContent(data string, content, reasoning *strings.Builder) {
	var chunk map[string]any
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}

	choices, ok := chunk["choices"].([]any)
	if !ok || len(choices) == 0 {
		return
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return
	}

	if delta, ok := choice["delta"].(map[string]any); ok {
		if c, ok := delta["content"].(string); ok {
			content.WriteString(c)
		}
		if r, ok := delta["reasoning_content"].(string); ok {
			reasoning.WriteString(r)
		}
	}

	if text, ok := choice["text"].(string); ok {
		content.WriteString(text)
	}
}

func (h *Handler) errorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "gateway_error",
		},
	})
}

func excludeEmbedding(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range data {
		if k == "embedding" || k == "data" {
			if arr, ok := v.([]any); ok && len(arr) > 0 {
				if item, ok := arr[0].(map[string]any); ok {
					if _, hasEmbedding := item["embedding"]; hasEmbedding {
						continue
					}
				}
			}
		}
		result[k] = v
	}
	return result
}
