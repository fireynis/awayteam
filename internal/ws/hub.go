package ws

import "sync"

type Hub struct {
	mu             sync.RWMutex
	clients        map[chan []byte]struct{}
	agentListeners map[string]chan []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:        make(map[chan []byte]struct{}),
		agentListeners: make(map[string]chan []byte),
	}
}

func (h *Hub) Register(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
}

func (h *Hub) Unregister(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) RegisterAgent(agentID string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agentListeners[agentID] = ch
}

func (h *Hub) UnregisterAgent(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.agentListeners[agentID]; ok {
		close(ch)
		delete(h.agentListeners, agentID)
	}
}

func (h *Hub) RouteResponse(agentID string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if ch, ok := h.agentListeners[agentID]; ok {
		select {
		case ch <- data:
		default:
		}
	}
}
