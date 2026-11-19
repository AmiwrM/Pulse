package app

import (
	"encoding/json"
	"sync"
)

type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *Hub {
	return &Hub{clients: map[chan []byte]struct{}{}}
}

func (h *Hub) subscribe() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) broadcast(event string, data any) {
	payload, err := json.Marshal(map[string]any{"type": event, "data": data, "at": nowUnix()})
	if err != nil {
		return
	}
	payload = append(payload, '\n')
	h.mu.Lock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
			// Slow consumer; it will refresh when it reconnects.
		}
	}
	h.mu.Unlock()
}
