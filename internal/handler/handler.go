package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LucienShui/openai-gateway/internal/config"
	"github.com/LucienShui/openai-gateway/internal/logger"
)

type Handler struct {
	cfg    *config.Config
	logger *logger.Logger
}

func New(cfg *config.Config, log *logger.Logger) *Handler {
	return &Handler{cfg: cfg, logger: log}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
	})
}

func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.cfg.ModelList)
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
	r.Body.Close()

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
		h.logger.Error(map[string]any{
			"api":        path,
			"error":      err.Error(),
			"request_id": requestID,
		})
		h.errorResponse(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		if k == "Content-Length" || k == "Transfer-Encoding" {
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	if isStream {
		h.handleStream(w, resp, path, reqBody, requestID, startTime)
	} else {
		h.handleNonStream(w, resp, path, reqBody, requestID, startTime)
	}
}

func (h *Handler) handleStream(w http.ResponseWriter, resp *http.Response, path string, reqBody map[string]any, requestID string, startTime time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error(map[string]any{
			"api":        path,
			"error":      "streaming not supported",
			"request_id": requestID,
		})
		return
	}

	var responseContent strings.Builder
	var reasoningContent strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		fmt.Fprintf(w, "%s\n\n", line)
		flusher.Flush()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}
			h.extractContent(data, &responseContent, &reasoningContent)
		}
	}

	logEntry := map[string]any{
		"api":      path,
		"request":  excludeEmbedding(reqBody),
		"response": responseContent.String(),
		"time":     time.Since(startTime).Seconds(),
	}
	if requestID != "" {
		logEntry["request_id"] = requestID
	}
	if reasoningContent.Len() > 0 {
		logEntry["reasoning_content"] = reasoningContent.String()
	}
	h.logger.Info(logEntry)
}

func (h *Handler) handleNonStream(w http.ResponseWriter, resp *http.Response, path string, reqBody map[string]any, requestID string, startTime time.Time) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.errorResponse(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	var respJSON map[string]any
	json.Unmarshal(respBody, &respJSON)

	logEntry := map[string]any{
		"api":      path,
		"request":  excludeEmbedding(reqBody),
		"response": excludeEmbedding(respJSON),
		"time":     time.Since(startTime).Seconds(),
	}
	if requestID != "" {
		logEntry["request_id"] = requestID
	}
	h.logger.Info(logEntry)
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
	json.NewEncoder(w).Encode(map[string]any{
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
