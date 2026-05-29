package ws

import (
	"encoding/json"
	"joki_roblox/internal/models"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Client merepresentasikan satu koneksi WebSocket dari browser pelanggan
type Client struct {
	passcode string
	conn     *websocket.Conn
	send     chan []byte
}

// Hub mengelola semua koneksi WebSocket yang aktif dan mendistribusikan pesan
type Hub struct {
	// clients menyimpan semua client aktif (key: passcode)
	// Satu passcode bisa memiliki lebih dari satu koneksi (misal buka di dua tab)
	clients map[string]map[*Client]bool

	// register adalah channel untuk mendaftarkan client baru
	register chan *Client

	// unregister adalah channel untuk menghapus client yang terputus
	unregister chan *Client

	mu sync.RWMutex
}

// NewHub membuat instance Hub baru
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
	}
}

// Run menjalankan event loop Hub di goroutine terpisah
// Harus dipanggil dengan: go hub.Run()
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if _, ok := h.clients[client.passcode]; !ok {
				h.clients[client.passcode] = make(map[*Client]bool)
			}
			h.clients[client.passcode][client] = true
			h.mu.Unlock()
			log.Printf("[WS Hub] Client terdaftar | passcode: %s | total: %d", client.passcode, h.countClients())

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.passcode]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.clients, client.passcode)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("[WS Hub] Client keluar | passcode: %s | total: %d", client.passcode, h.countClients())
		}
	}
}

// BroadcastToPasscode mengirimkan pesan real-time ke semua browser yang
// sedang membuka halaman tracking dengan passcode tertentu.
// Dipanggil saat admin mengupdate status atau mengupload screenshot.
func (h *Hub) BroadcastToPasscode(passcode string, msg *models.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS Hub] Gagal marshal pesan: %v", err)
		return
	}

	h.mu.RLock()
	clients := h.clients[passcode]
	h.mu.RUnlock()

	sent := 0
	for client := range clients {
		select {
		case client.send <- data:
			sent++
		default:
			// Buffer penuh, hapus client ini
			h.unregister <- client
		}
	}

	if sent > 0 {
		log.Printf("[WS Hub] Broadcast ke %d client | passcode: %s", sent, passcode)
	} else {
		log.Printf("[WS Hub] Tidak ada client aktif untuk passcode: %s (update tersimpan di DB)", passcode)
	}
}

// ServeWS mengupgrade koneksi HTTP menjadi WebSocket dan mendaftarkan client ke Hub.
// Fungsi ini blocking (menjalankan readPump), dipanggil dalam goroutine dari handler HTTP.
func (h *Hub) ServeWS(upgrader *websocket.Upgrader, passcode string, conn *websocket.Conn) {
	client := &Client{
		passcode: passcode,
		conn:     conn,
		send:     make(chan []byte, 32),
	}

	h.register <- client

	// Goroutine terpisah untuk mengirim pesan ke browser
	go client.writePump(h)

	// Blocking: membaca dari browser & mendeteksi pemutusan koneksi
	client.readPump(h)
}

// readPump membaca pesan dari client dan mendeteksi penutupan koneksi.
// Pelanggan tidak perlu mengirim data ke server, ini hanya untuk keep-alive detection.
func (c *Client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	// Batasi ukuran pesan yang bisa diterima dari client (cukup kecil)
	c.conn.SetReadLimit(512)

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				log.Printf("[WS Client] Koneksi terputus tidak terduga: %v", err)
			}
			break
		}
	}
}

// writePump mengambil pesan dari channel send dan menulisnya ke koneksi WebSocket client.
func (c *Client) writePump(h *Hub) {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("[WS Client] Gagal kirim pesan: %v", err)
			return
		}
	}
}

// countClients menghitung total seluruh client WebSocket yang terhubung saat ini
func (h *Hub) countClients() int {
	total := 0
	for _, clients := range h.clients {
		total += len(clients)
	}
	return total
}
