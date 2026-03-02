package ws

import (
	"testing"
	"time"
)

func TestHub_RegisterAndBroadcast(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 256)
	h.Register(ch)

	if h.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", h.ClientCount())
	}

	h.Broadcast([]byte("hello"))

	select {
	case msg := <-ch:
		if string(msg) != "hello" {
			t.Fatalf("expected 'hello', got '%s'", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestHub_Unregister(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 256)
	h.Register(ch)
	h.Unregister(ch)

	if h.ClientCount() != 0 {
		t.Fatalf("expected 0 clients, got %d", h.ClientCount())
	}
}

func TestHub_BroadcastSkipsFullChannel(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 1)
	h.Register(ch)

	ch <- []byte("fill")

	done := make(chan struct{})
	go func() {
		h.Broadcast([]byte("overflow"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on full channel")
	}
}

func TestHub_AgentRouteResponse(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 256)
	h.RegisterAgent("agent-1", ch)

	h.RouteResponse("agent-1", []byte("response"))

	select {
	case msg := <-ch:
		if string(msg) != "response" {
			t.Fatalf("expected 'response', got '%s'", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for routed response")
	}
}

func TestHub_RouteResponseUnknownAgent(t *testing.T) {
	// Should not panic
	h := NewHub()

	h.RouteResponse("nonexistent", []byte("test"))
}
