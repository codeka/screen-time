package realtime

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev/PWA
	},
}

type Hub struct {
	mu           sync.RWMutex
	parentConns  map[string]map[*websocket.Conn]bool // familyID -> conns
	deviceConns  map[string]map[*websocket.Conn]bool // deviceID -> conns
}

func NewHub() *Hub {
	return &Hub{
		parentConns: make(map[string]map[*websocket.Conn]bool),
		deviceConns: make(map[string]map[*websocket.Conn]bool),
	}
}

func (h *Hub) RegisterParent(familyID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.parentConns[familyID]; !ok {
		h.parentConns[familyID] = make(map[*websocket.Conn]bool)
	}
	h.parentConns[familyID][conn] = true
	log.Printf("[WS] Parent connected for family %s", familyID)
}

func (h *Hub) UnregisterParent(familyID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.parentConns[familyID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.parentConns, familyID)
		}
	}
	conn.Close()
	log.Printf("[WS] Parent disconnected for family %s", familyID)
}

func (h *Hub) RegisterDevice(deviceID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.deviceConns[deviceID]; !ok {
		h.deviceConns[deviceID] = make(map[*websocket.Conn]bool)
	}
	h.deviceConns[deviceID][conn] = true
	log.Printf("[WS] Device connected: %s", deviceID)
}

func (h *Hub) UnregisterDevice(deviceID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.deviceConns[deviceID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.deviceConns, deviceID)
		}
	}
	conn.Close()
	log.Printf("[WS] Device disconnected: %s", deviceID)
}

func (h *Hub) BroadcastToFamily(familyID string, message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	h.mu.RLock()
	conns := make([]*websocket.Conn, 0)
	if familyMap, ok := h.parentConns[familyID]; ok {
		for c := range familyMap {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()

	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WS] Failed to write to parent conn: %v", err)
		}
	}
}

func (h *Hub) SendToDevice(deviceID string, message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	h.mu.RLock()
	conns := make([]*websocket.Conn, 0)
	if devMap, ok := h.deviceConns[deviceID]; ok {
		for c := range devMap {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()

	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WS] Failed to write to device conn: %v", err)
		}
	}
}

func (h *Hub) Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return upgrader.Upgrade(w, r, nil)
}

