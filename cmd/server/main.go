package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"joki_roblox/internal/db"
	"joki_roblox/internal/models"
	"joki_roblox/internal/ws"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/SherClockHolmes/webpush-go"
)

// =====================
// SETUP & CONFIG
// =====================

var (
	database        *db.DB
	hub             *ws.Hub
	adminPassword   string
	vapidPublicKey  string
	vapidPrivateKey string
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Izinkan semua origin (untuk pengembangan lokal & akses dari HP)
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	setupWorkDir()

	// Load konfigurasi dari file .env
	if err := godotenv.Load(); err != nil {
		log.Println("[Config] File .env tidak ditemukan, menggunakan nilai default...")
	}

	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "./joki.db")
	adminPassword = getEnv("ADMIN_PASSWORD", "admin123")
	uploadsDir := "./public/uploads"

	// Load atau generate VAPID keys untuk Web Push
	vapidPublicKey = getEnv("VAPID_PUBLIC_KEY", "")
	vapidPrivateKey = getEnv("VAPID_PRIVATE_KEY", "")
	if vapidPublicKey == "" || vapidPrivateKey == "" {
		log.Println("[WebPush] VAPID keys tidak ditemukan di .env. Membuat key baru...")
		pub, priv, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			log.Printf("[WebPush] Gagal generate VAPID keys: %v", err)
		} else {
			vapidPublicKey = pub
			vapidPrivateKey = priv
			log.Println("[WebPush] ==========================================================")
			log.Println("[WebPush] BERHASIL GENERATE VAPID KEYS!")
			log.Printf("[WebPush] VAPID_PUBLIC_KEY  = %s", vapidPublicKey)
			log.Printf("[WebPush] VAPID_PRIVATE_KEY = %s", vapidPrivateKey)
			log.Println("[WebPush] Silakan simpan key di atas ke file .env VPS/Server Anda!")
			log.Println("[WebPush] ==========================================================")
		}
	}

	// Inisialisasi database
	var err error
	database, err = db.New(dbPath)
	if err != nil {
		log.Fatalf("[FATAL] Gagal inisialisasi database: %v", err)
	}
	defer database.Close()

	// Pastikan direktori uploads ada
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Fatalf("[FATAL] Gagal membuat direktori uploads: %v", err)
	}

	// Inisialisasi & jalankan WebSocket Hub
	hub = ws.NewHub()
	go hub.Run()

	// Setup HTTP router
	mux := http.NewServeMux()

	// --- Static file server untuk hasil build Next.js ---
	// Saat frontend sudah di-build, file akan tersimpan di ./frontend/out/
	frontendPath := "./frontend/out"
	if _, err := os.Stat(frontendPath); err == nil {
		fileServer := http.FileServer(http.Dir(frontendPath))
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Cegah cache untuk halaman utama (HTML) agar setiap ada update kode langsung berubah
			if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
			} else if strings.HasPrefix(r.URL.Path, "/_next/static/") {
				// File JS/CSS build dari Next.js aman di-cache lama
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
		}))
		log.Printf("[Server] Menyajikan frontend dari: %s", frontendPath)
	} else {
		// Fallback: tampilkan halaman sementara jika frontend belum di-build
		mux.HandleFunc("/", handlerRoot)
	}

	// Static file uploads (screenshot joki)
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir))))

	// --- Endpoint Pelanggan (Public) ---
	mux.HandleFunc("/api/order", handlerGetOrder)       // GET /api/order?passcode=BJ-1234
	mux.HandleFunc("/api/roblox-avatar", handlerRobloxAvatar) // GET /api/roblox-avatar?username=evosterbaik
	mux.HandleFunc("/api/vapid-public-key", handlerVapidPublicKey) // GET /api/vapid-public-key
	mux.HandleFunc("/api/save-subscription", handlerSaveSubscription) // POST /api/save-subscription
	mux.HandleFunc("/ws", handlerWebSocket)             // GET /ws?passcode=BJ-1234

	// --- Endpoint Admin (Protected) ---
	mux.HandleFunc("/api/admin/login", handlerAdminLogin)     // POST /api/admin/login
	mux.HandleFunc("/api/admin/logout", handlerAdminLogout)   // POST /api/admin/logout
	mux.HandleFunc("/api/admin/orders", adminMiddleware(handlerAdminListOrders)) // GET  /api/admin/orders
	mux.HandleFunc("/api/admin/orders/create", adminMiddleware(handlerAdminCreateOrder)) // POST /api/admin/orders/create
	mux.HandleFunc("/api/admin/orders/delete", adminMiddleware(handlerAdminDeleteOrder)) // POST /api/admin/orders/delete
	mux.HandleFunc("/api/admin/status", adminMiddleware(handlerAdminUpdateStatus)) // POST /api/admin/status
	mux.HandleFunc("/api/admin/upload", adminMiddleware(handlerAdminUpload))     // POST /api/admin/upload

	log.Printf("╔══════════════════════════════════════════╗")
	log.Printf("║   🎮 Joki Roblox Server - Siap!          ║")
	log.Printf("║   Port  : %s                           ║", port)
	log.Printf("║   DB    : %s                       ║", dbPath)
	log.Printf("╚══════════════════════════════════════════╝")

	if err := http.ListenAndServe(":"+port, corsMiddleware(mux)); err != nil {
		log.Fatalf("[FATAL] Server error: %v", err)
	}
}

// =====================
// MIDDLEWARE
// =====================

// corsMiddleware menambahkan header CORS agar frontend Next.js bisa berkomunikasi dengan API
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Cookie, X-Admin-Password")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminMiddleware memvalidasi token admin dari cookie (web) ATAU password dari header X-Admin-Password (CLI)
func adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Cek Cookie admin_token (untuk Web Admin)
		cookie, err := r.Cookie("admin_token")
		if err == nil && cookie.Value == generateAdminToken() {
			next(w, r)
			return
		}

		// 2. Cek Header X-Admin-Password (untuk CLI Lokal Laptop)
		cliPass := r.Header.Get("X-Admin-Password")
		if cliPass != "" && cliPass == adminPassword {
			next(w, r)
			return
		}

		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Akses ditolak. Token tidak valid atau password salah.",
		})
	}
}

// =====================
// HANDLER - PELANGGAN
// =====================

// handlerRoot adalah halaman fallback sementara sebelum frontend Next.js di-build
func handlerRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <title>Joki Roblox - Backend Aktif</title>
  <style>
    body { font-family: monospace; background: #0B0813; color: #A855F7; display: flex;
           justify-content: center; align-items: center; min-height: 100vh; margin: 0; }
    .box { border: 1px solid #A855F7; padding: 40px; border-radius: 12px;
           background: rgba(168,85,247,0.05); text-align: center; }
    h1 { font-size: 2rem; } p { color: #9CA3AF; }
    code { background: rgba(168,85,247,0.2); padding: 4px 8px; border-radius: 4px; }
  </style>
</head>
<body>
  <div class="box">
    <h1>🎮 Joki Roblox Backend</h1>
    <p>Server Go aktif & berjalan dengan baik!</p>
    <p>API tersedia di <code>/api/order?passcode=...</code></p>
    <p>WebSocket tersedia di <code>/ws?passcode=...</code></p>
    <p>Build frontend Next.js dan tempatkan hasilnya di <code>./frontend/out/</code></p>
  </div>
</body>
</html>`)
}

// handlerGetOrder mengambil detail order berdasarkan passcode (untuk pelanggan)
// GET /api/order?passcode=BJ-1234
func handlerGetOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	passcode := strings.TrimSpace(r.URL.Query().Get("passcode"))
	if passcode == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Parameter 'passcode' wajib diisi"})
		return
	}

	detail, err := database.GetOrderByPasscode(passcode)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan server"})
		return
	}
	if detail == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Passcode tidak ditemukan. Pastikan kode yang kamu masukkan sudah benar."})
		return
	}

	respondJSON(w, http.StatusOK, detail)
}

// handlerWebSocket mengupgrade koneksi HTTP ke WebSocket untuk live update
// GET /ws?passcode=BJ-1234
func handlerWebSocket(w http.ResponseWriter, r *http.Request) {
	passcode := strings.TrimSpace(r.URL.Query().Get("passcode"))
	if passcode == "" {
		http.Error(w, "Parameter 'passcode' wajib diisi", http.StatusBadRequest)
		return
	}

	// Verifikasi passcode valid sebelum upgrade ke WebSocket
	detail, err := database.GetOrderByPasscode(passcode)
	if err != nil || detail == nil {
		http.Error(w, "Passcode tidak valid", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Gagal upgrade koneksi: %v", err)
		return
	}

	// Kirimkan state terkini segera setelah koneksi terbuka
	go func() {
		time.Sleep(100 * time.Millisecond) // beri waktu untuk writePump siap
		hub.BroadcastToPasscode(passcode, &models.WSMessage{
			Type:    "order_update",
			Payload: detail,
		})
	}()

	hub.ServeWS(&upgrader, passcode, conn)
}

// =====================
// HANDLER - ADMIN
// =====================

// handlerAdminLogin memvalidasi password admin dan menyimpan token sesi ke cookie
// POST /api/admin/login
func handlerAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	var req models.AdminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Format body tidak valid"})
		return
	}

	if req.Password != adminPassword {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Password admin salah"})
		return
	}

	// Simpan token sesi ke cookie (berlaku 24 jam)
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    generateAdminToken(),
		Path:     "/",
		MaxAge:   86400, // 24 jam
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	respondJSON(w, http.StatusOK, map[string]string{"message": "Login berhasil! Selamat datang, Admin."})
}

// handlerAdminLogout menghapus cookie token sesi admin dari browser secara aman
// POST /api/admin/logout
func handlerAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Hapus cookie
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	respondJSON(w, http.StatusOK, map[string]string{"message": "Logout berhasil"})
}

// handlerAdminListOrders mengembalikan daftar semua order untuk dropdown di halaman admin HP
// GET /api/admin/orders
func handlerAdminListOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	orders, err := database.ListOrders()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data order"})
		return
	}
	if orders == nil {
		orders = []models.Order{}
	}
	respondJSON(w, http.StatusOK, orders)
}

// handlerAdminUpdateStatus memperbarui status joki dan trigger WebSocket broadcast ke pelanggan
// POST /api/admin/status
func handlerAdminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	var req models.UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Format body tidak valid"})
		return
	}

	if req.Passcode == "" || req.Status == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Field 'passcode' dan 'status' wajib diisi"})
		return
	}

	// Validasi status
	validStatus := map[models.OrderStatus]bool{
		models.StatusPending:    true,
		models.StatusQueued:     true,
		models.StatusInProgress: true,
		models.StatusCompleted:  true,
	}
	if !validStatus[req.Status] {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Status '%s' tidak valid. Pilihan: pending, queued, in_progress, completed", req.Status),
		})
		return
	}

	// Update di database
	if err := database.UpdateOrderStatus(req.Passcode, req.Status, req.Notes); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// Broadcast update real-time ke browser pelanggan yang sedang terbuka
	detail, _ := database.GetOrderByPasscode(req.Passcode)
	if detail != nil {
		hub.BroadcastToPasscode(req.Passcode, &models.WSMessage{
			Type:    "order_update",
			Payload: detail,
		})

		// Kirim push notification ke HP/browser latar belakang
		statusLabel := map[models.OrderStatus]string{
			models.StatusPending:    "Menunggu Pengerjaan",
			models.StatusQueued:     "Masuk Antrean",
			models.StatusInProgress: "Sedang Diproses",
			models.StatusCompleted:  "Selesai!",
		}
		title := "Update Joki: " + detail.RobloxUsername
		body := "Status joki berubah menjadi " + statusLabel[detail.Status]
		if req.Notes != "" {
			body += " · " + req.Notes
		}
		sendWebPush(detail.ID, title, body)
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Status order %s berhasil diubah ke '%s'", req.Passcode, req.Status),
	})
}

// handlerAdminUpload menerima upload screenshot dari HP admin dan broadcast ke pelanggan
// POST /api/admin/upload
// Form: multipart/form-data dengan field "passcode" dan "screenshot" (file)
func handlerAdminUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	// Batasi ukuran upload maks 10 MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ukuran file terlalu besar (maks 10MB)"})
		return
	}

	passcode := strings.TrimSpace(r.FormValue("passcode"))
	if passcode == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Field 'passcode' wajib diisi"})
		return
	}

	// Verifikasi order ada
	orderID, err := database.GetOrderIDByPasscode(passcode)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// Ambil file upload
	file, header, err := r.FormFile("screenshot")
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "File 'screenshot' tidak ditemukan dalam form"})
		return
	}
	defer file.Close()

	// Validasi ekstensi file (hanya gambar)
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExt := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
	if !allowedExt[ext] {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Tipe file tidak didukung. Gunakan: jpg, jpeg, png, gif, atau webp"})
		return
	}

	// Generate nama file unik agar tidak tumpang tindih
	filename := fmt.Sprintf("%s_%d%s", sanitizeFilename(passcode), time.Now().UnixMilli(), ext)
	savePath := filepath.Join("./public/uploads", filename)

	// Simpan file ke disk
	dst, err := os.Create(savePath)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan file ke server"})
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal menulis file"})
		return
	}

	// Simpan referensi screenshot ke database
	screenshot := &models.Screenshot{
		OrderID:  orderID,
		Filename: filename,
	}
	if err := database.AddScreenshot(screenshot); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan data screenshot ke database"})
		return
	}
	screenshot.URL = "/uploads/" + filename

	// Broadcast screenshot baru ke browser pelanggan secara real-time
	hub.BroadcastToPasscode(passcode, &models.WSMessage{
		Type:    "new_screenshot",
		Payload: screenshot,
	})

	// Kirim push notification ke HP/browser latar belakang
	orderDetail, _ := database.GetOrderByPasscode(passcode)
	if orderDetail != nil {
		title := "Screenshot Baru! 📸"
		body := fmt.Sprintf("Admin baru saja mengunggah screenshot live terbaru untuk joki %s.", orderDetail.RobloxUsername)
		sendWebPush(orderDetail.ID, title, body)
	}

	log.Printf("[Upload] Screenshot baru: %s untuk passcode: %s", filename, passcode)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Screenshot berhasil diunggah!",
		"screenshot": screenshot,
	})
}

// handlerAdminCreateOrder membuat order baru secara remote dari CLI
// POST /api/admin/orders/create
func handlerAdminCreateOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	var req struct {
		RobloxUsername string `json:"roblox_username"`
		PackageName    string `json:"package_name"`
		Price          int    `json:"price"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Format body tidak valid"})
		return
	}

	if req.RobloxUsername == "" || req.PackageName == "" || req.Price <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Field 'roblox_username', 'package_name', dan 'price' wajib diisi dengan benar"})
		return
	}

	// Buat passcode acak
	passcode := generatePasscode()

	order := &models.Order{
		Passcode:       passcode,
		RobloxUsername: req.RobloxUsername,
		PackageName:    models.PackageName(req.PackageName),
		Status:         models.StatusPending,
		Price:          req.Price,
	}

	if err := database.CreateOrder(order); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Gagal membuat order: %v", err)})
		return
	}

	log.Printf("[Server] Order baru dibuat secara remote: %s untuk %s", passcode, req.RobloxUsername)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Order berhasil dibuat!",
		"order":   order,
	})
}

// handlerAdminDeleteOrder menghapus order berdasarkan passcode secara remote dari CLI
// POST /api/admin/orders/delete
func handlerAdminDeleteOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	var req struct {
		Passcode string `json:"passcode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Format body tidak valid"})
		return
	}

	if req.Passcode == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Field 'passcode' wajib diisi"})
		return
	}

	if err := database.DeleteOrder(req.Passcode); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("[Server] Order %s berhasil dihapus secara remote", req.Passcode)

	respondJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Order %s berhasil dihapus!", req.Passcode),
	})
}

// generatePasscode membuat kode unik acak dalam format BJ-XXXX
func generatePasscode() string {
	seed := time.Now().UnixNano()
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Hindari 0,O,I,1 yang mudah tertukar
	code := make([]byte, 4)
	for i := range code {
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		code[i] = chars[seed%int64(len(chars))]
	}
	return fmt.Sprintf("BJ-%s", string(code))
}

// =====================
// HELPER FUNCTIONS
// =====================

// respondJSON menulis response HTTP dalam format JSON
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[Server] Gagal encode JSON response: %v", err)
	}
}

// getEnv mengambil nilai environment variable atau mengembalikan nilai default
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// generateAdminToken membuat token sesi sederhana berbasis password
// (Untuk produksi, gunakan JWT atau token acak yang disimpan di memory)
func generateAdminToken() string {
	return fmt.Sprintf("admin-%x", []byte(adminPassword))
}

// sanitizeFilename membersihkan string passcode agar aman dijadikan nama file
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// setupWorkDir auto-detects the project root (where go.mod is located)
// and shifts the working directory to it so relative paths are always valid.
func setupWorkDir() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	dir := wd
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if dir != wd {
				if err := os.Chdir(dir); err == nil {
					log.Printf("[System] Working directory disesuaikan ke root project: %s", dir)
				}
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

// handlerRobloxAvatar proxies requests to Roblox API to avoid CORS and rate limits on shared public proxies
// GET /api/roblox-avatar?username=evosterbaik
func handlerRobloxAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Parameter 'username' wajib diisi"})
		return
	}

	// 1. Dapatkan UserID dari Roblox secara server-side
	userID, err := fetchRobloxUserID(username)
	if err != nil {
		log.Printf("[Roblox Proxy] Gagal mencari User ID untuk %s: %v", username, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal terhubung ke API Roblox"})
		return
	}
	if userID == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "User tidak ditemukan"})
		return
	}

	// 2. Dapatkan headshot avatar URL secara server-side
	avatarURL, err := fetchRobloxAvatarURL(userID)
	if err != nil {
		log.Printf("[Roblox Proxy] Gagal mengambil Avatar Headshot untuk UserID %d: %v", userID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil foto profil Roblox"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"avatar_url": avatarURL})
}

// fetchRobloxUserID mengambil UserID Roblox berdasarkan username secara server-side
func fetchRobloxUserID(username string) (int64, error) {
	url := "https://users.roblox.com/v1/usernames/users"
	requestBody, err := json.Marshal(map[string]interface{}{
		"usernames":          []string{username},
		"excludeBannedUsers": false,
	})
	if err != nil {
		return 0, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("roblox api error: status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	if len(result.Data) == 0 {
		return 0, nil
	}

	return result.Data[0].ID, nil
}

// fetchRobloxAvatarURL mengambil headshot avatar URL Roblox berdasarkan UserID secara server-side
func fetchRobloxAvatarURL(userID int64) (string, error) {
	url := fmt.Sprintf("https://thumbnails.roblox.com/v1/users/avatar-headshot?userIds=%d&size=150x150&format=Png&isCircular=true", userID)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("roblox thumbnail api error: status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ImageURL string `json:"imageUrl"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Data) == 0 {
		return "", nil
	}

	return result.Data[0].ImageURL, nil
}

// handlerVapidPublicKey mengembalikan VAPID public key untuk frontend browser
// GET /api/vapid-public-key
func handlerVapidPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"public_key": vapidPublicKey})
}

// handlerSaveSubscription menyimpan detail langganan push dari browser pelanggan
// POST /api/save-subscription
func handlerSaveSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method tidak diizinkan"})
		return
	}

	var req models.SaveSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Format body tidak valid"})
		return
	}

	if req.Passcode == "" || req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Field passcode, endpoint, p256dh, dan auth wajib diisi"})
		return
	}

	// Cari order ID berdasarkan passcode
	orderID, err := database.GetOrderIDByPasscode(req.Passcode)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Passcode tidak ditemukan"})
		return
	}

	// Simpan ke database
	err = database.AddSubscription(orderID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth)
	if err != nil {
		log.Printf("[WebPush] Gagal menyimpan subscription ke DB: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal menyimpan subscription"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Subscription berhasil didaftarkan!"})
}

// sendWebPush mengirimkan push notification native ke semua HP/browser yang terdaftar untuk orderID tersebut
func sendWebPush(orderID int, title, body string) {
	if vapidPublicKey == "" || vapidPrivateKey == "" {
		log.Println("[WebPush] VAPID keys kosong, tidak dapat mengirim notifikasi push.")
		return
	}

	subs, err := database.GetSubscriptionsByOrderID(orderID)
	if err != nil {
		log.Printf("[WebPush] Gagal mengambil subscription dari DB untuk order %d: %v", orderID, err)
		return
	}

	if len(subs) == 0 {
		return
	}

	log.Printf("[WebPush] Mengirim notifikasi push ke %d device untuk order %d...", len(subs), orderID)

	payload, err := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
	})
	if err != nil {
		log.Printf("[WebPush] Gagal encode payload push: %v", err)
		return
	}

	for _, sub := range subs {
		s := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256dh,
				Auth:   sub.Auth,
			},
		}

		go func(subObj *webpush.Subscription, endpointStr string) {
			resp, err := webpush.SendNotification(payload, subObj, &webpush.Options{
				Subscriber:      "mailto:billystorepanel@gmail.com",
				VAPIDPublicKey:  vapidPublicKey,
				VAPIDPrivateKey: vapidPrivateKey,
				TTL:             86400, // 24 jam
			})
			if err != nil {
				log.Printf("[WebPush] Gagal kirim push ke %s: %v", endpointStr, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
				log.Printf("[WebPush] Device %s sudah tidak aktif (status %d), menghapus subscription...", endpointStr, resp.StatusCode)
				database.DeleteSubscriptionByEndpoint(endpointStr)
			}
		}(s, sub.Endpoint)
	}
}
