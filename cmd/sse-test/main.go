package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/tmaxmax/go-sse"
)

const (
	defaultBaseURL = "https://api.siliconflow.cn/v1"
	defaultModel   = "Qwen/Qwen3-30B-A3B-Thinking-2507"
)

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionChunk struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Delta Delta `json:"delta"`
}

type Delta struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

func main() {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "API_KEY environment variable is required")
		os.Exit(1)
	}

	// Build request body
	chatReq := ChatRequest{
		Model: defaultModel,
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Stream: true,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal request: %v\n", err)
		os.Exit(1)
	}

	// Create HTTP request
	url := baseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Use go-sse client to handle the stream
	client := sse.Client{
		HTTPClient: http.DefaultClient,
		// Accept any response (OpenAI returns application/json for errors)
		ResponseValidator: sse.NoopValidator,
		// Disable automatic reconnection - we only want one request
		Backoff: sse.Backoff{
			MaxRetries: -1, // No retries
		},
	}

	conn := client.NewConnection(req)

	// Subscribe to all messages
	conn.SubscribeMessages(func(event sse.Event) {
		data := event.Data

		// Check for [DONE] signal
		if data == "[DONE]" {
			return
		}

		// Parse the JSON chunk
		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return
		}

		if len(chunk.Choices) == 0 {
			return
		}

		delta := chunk.Choices[0].Delta

		// Print reasoning_content if present
		if delta.ReasoningContent != "" {
			fmt.Printf("reasoning_content: %s\n", delta.ReasoningContent)
		}

		// Print content if present
		if delta.Content != "" {
			fmt.Printf("content: %s\n", delta.Content)
		}
	})

	// Connect and stream
	err = conn.Connect()
	// EOF is expected when server closes connection after [DONE]
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
}
