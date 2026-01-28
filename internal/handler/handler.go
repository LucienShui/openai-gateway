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
	err := json.NewEncoder(w).Encode(h.cfg.ModelList)
	if err != nil {
		http.Error(w, "encode models failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) Stream(w http.ResponseWriter, url string, body []byte, apiKey string) {
	upstreamReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		http.Error(w, "failed to connect to upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			h.logger.Error(map[string]any{
				"error": "body close failed",
			})
		}
	}(resp.Body)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				http.Error(w, "failed to connect to upstream: "+err.Error(), http.StatusInternalServerError)
			}
			break
		}
		_, err = w.Write(line)
		if err != nil {
			http.Error(w, "streaming write error: "+err.Error(), http.StatusInternalServerError)
		}
		flusher.Flush()
	}
}

func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {}

func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	err = r.Body.Close()
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "failed to close request body")
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
	upstreamBody, _ := json.Marshal(reqBody)
	upstreamURL := route.Client.BuildURL(path)
	isStream, _ := reqBody["stream"].(bool)

	if isStream {
		h.Stream(w, upstreamURL, upstreamBody, route.Client.APIKey)
	} else {
		//
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
