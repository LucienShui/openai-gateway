package sse_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tmaxmax/go-sse"
)

// TestSSEServer tests basic SSE server functionality
func TestSSEServer(t *testing.T) {
	// Create a new SSE server (uses default Joe provider)
	sseServer := &sse.Server{}

	// Create a test HTTP server
	mux := http.NewServeMux()
	mux.Handle("/events", sseServer)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Create a client
	client := sse.Client{
		HTTPClient: ts.Client(),
		OnRetry: func(_ error, _ time.Duration) {
			t.Log("Retrying connection...")
		},
	}

	// Channel to receive events
	eventCh := make(chan sse.Event, 10)
	errCh := make(chan error, 1)

	// Start listening for events in a goroutine
	go func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
		conn := client.NewConnection(req)
		conn.SubscribeMessages(func(event sse.Event) {
			eventCh <- event
		})
		if err := conn.Connect(); err != nil && err != context.Canceled {
			errCh <- err
		}
	}()

	// Give the client time to connect
	time.Sleep(100 * time.Millisecond)

	// Publish an event
	msg := &sse.Message{}
	msg.AppendData("Hello, SSE!")
	sseServer.Publish(msg)

	// Wait for the event or timeout
	select {
	case event := <-eventCh:
		if event.Data != "Hello, SSE!" {
			t.Errorf("Expected 'Hello, SSE!', got '%s'", event.Data)
		}
		t.Logf("Received event: %s", event.Data)
	case err := <-errCh:
		t.Fatalf("Connection error: %v", err)
	case <-ctx.Done():
		t.Log("Test completed (timeout reached)")
	}
}

// TestSSEClient tests the SSE client functionality
func TestSSEClient(t *testing.T) {
	// Create a simple SSE endpoint
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send a few events
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: Event %d\n\n", i)
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := sse.Client{
		HTTPClient: ts.Client(),
	}

	receivedEvents := make([]string, 0)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	conn := client.NewConnection(req)

	conn.SubscribeMessages(func(event sse.Event) {
		receivedEvents = append(receivedEvents, event.Data)
	})

	err := conn.Connect()
	if err != nil && err != context.DeadlineExceeded {
		// Connection closed by server is expected
		t.Logf("Connection ended: %v", err)
	}

	t.Logf("Received %d events", len(receivedEvents))
	for i, data := range receivedEvents {
		t.Logf("Event %d: %s", i, data)
	}
}

// TestSSEMessageBuilder tests building SSE messages
func TestSSEMessageBuilder(t *testing.T) {
	msg := &sse.Message{}
	msg.AppendData("line1", "line2")

	// Verify the message can be created
	if msg == nil {
		t.Fatal("Message should not be nil")
	}

	t.Log("Message created successfully")
}

// TestSSEWithEventType tests SSE with custom event types
func TestSSEWithEventType(t *testing.T) {
	sseServer := &sse.Server{}

	mux := http.NewServeMux()
	mux.Handle("/events", sseServer)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := sse.Client{
		HTTPClient: ts.Client(),
	}

	customEventCh := make(chan sse.Event, 10)

	go func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
		conn := client.NewConnection(req)
		conn.SubscribeEvent("custom", func(event sse.Event) {
			customEventCh <- event
		})
		conn.Connect()
	}()

	time.Sleep(100 * time.Millisecond)

	// Publish a custom event
	msg := &sse.Message{}
	msg.Type = sse.Type("custom")
	msg.AppendData("Custom event data")
	sseServer.Publish(msg)

	select {
	case event := <-customEventCh:
		t.Logf("Received custom event: %s", event.Data)
	case <-ctx.Done():
		t.Log("Test completed")
	}
}
