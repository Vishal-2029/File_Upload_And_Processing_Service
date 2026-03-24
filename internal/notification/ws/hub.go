package ws

import (
	"sync"

	"github.com/rs/zerolog/log"
)

// Hub maintains the set of active WebSocket clients and routes messages.
// All map mutations happen inside the single Run() goroutine — no mutex needed
// on the hot path. Send() uses RLock only for reading the clients map.
type Hub struct {
	// clients maps userID → set of connected clients
	clients map[string]map[*Client]struct{}
	mu      sync.RWMutex

	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
	stop       chan struct{}
}

// Message is the unit routed through the hub.
type Message struct {
	UserID  string
	Payload []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]struct{}),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan Message, 4096),
		stop:       make(chan struct{}),
	}
}

// Run processes register/unregister/broadcast in a single goroutine.
// Call this in a dedicated goroutine: go hub.Run()
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			if h.clients[c.userID] == nil {
				h.clients[c.userID] = make(map[*Client]struct{})
			}
			h.clients[c.userID][c] = struct{}{}
			h.mu.Unlock()
			log.Debug().Str("user_id", c.userID).Msg("ws client registered")

		case c := <-h.unregister:
			h.mu.Lock()
			if conns, ok := h.clients[c.userID]; ok {
				delete(conns, c)
				if len(conns) == 0 {
					delete(h.clients, c.userID)
				}
			}
			h.mu.Unlock()
			close(c.send)
			log.Debug().Str("user_id", c.userID).Msg("ws client unregistered")

		case m := <-h.broadcast:
			h.mu.RLock()
			conns := h.clients[m.UserID]
			h.mu.RUnlock()
			for c := range conns {
				select {
				case c.send <- m.Payload:
				default:
					log.Warn().Str("user_id", m.UserID).Msg("ws send buffer full, dropping message")
				}
			}

		case <-h.stop:
			return
		}
	}
}

// Send enqueues a message for delivery to all clients of userID.
// Non-blocking: drops the message if the broadcast channel is full.
func (h *Hub) Send(userID string, payload []byte) {
	select {
	case h.broadcast <- Message{UserID: userID, Payload: payload}:
	default:
		log.Warn().Str("user_id", userID).Msg("hub broadcast channel full, dropping message")
	}
}

func (h *Hub) Register(c *Client) {
	h.register <- c
}

func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

func (h *Hub) Stop() {
	close(h.stop)
}
