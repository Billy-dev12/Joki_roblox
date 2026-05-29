package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// Warna ANSI untuk tampilan terminal yang menarik
const (
	colorReset  = "\033[0m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
)

var (
	serverURL     string
	adminPassword string
)

type Order struct {
	ID             int       `json:"id"`
	Passcode       string    `json:"passcode"`
	RobloxUsername string    `json:"roblox_username"`
	PackageName    string    `json:"package_name"`
	Status         string    `json:"status"`
	Price          int       `json:"price"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func main() {
	setupWorkDir()

	// Load konfigurasi dari .env lokal laptop
	_ = godotenv.Load()
	serverURL = getEnv("SERVER_URL", "http://localhost:8080")
	serverURL = strings.TrimSuffix(serverURL, "/")
	adminPassword = getEnv("ADMIN_PASSWORD", "adminroblox123")

	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	cmd := strings.ToLower(os.Args[1])
	switch cmd {
	case "add":
		cmdAdd()
	case "list":
		cmdList()
	case "status":
		cmdStatus()
	case "delete":
		cmdDelete()
	case "watch":
		cmdWatch()
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Printf("%s[Error]%s Perintah '%s' tidak dikenali.\n", colorRed, colorReset, cmd)
		printHelp()
		os.Exit(1)
	}
}

// =====================
// COMMAND: add
// =====================
func cmdAdd() {
	if len(os.Args) < 5 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: joki add <roblox_username> <nomor_paket> <harga>")
		fmt.Println()
		fmt.Println("  Pilihan Paket:")
		fmt.Println("    1. Pull Lever")
		fmt.Println("    2. Upgrade Race V4")
		fmt.Println()
		fmt.Println("  Contoh: joki add NarutoUzumaki 1 50000")
		os.Exit(1)
	}

	username := strings.TrimSpace(os.Args[2])
	pkgNum, err := strconv.Atoi(os.Args[3])
	if err != nil || pkgNum < 1 || pkgNum > 2 {
		fmt.Printf("%s[Error]%s Nomor paket tidak valid. Pilih 1 atau 2.\n", colorRed, colorReset)
		os.Exit(1)
	}

	price, err := strconv.Atoi(os.Args[4])
	if err != nil || price <= 0 {
		fmt.Printf("%s[Error]%s Harga harus berupa angka positif (dalam Rupiah).\n", colorRed, colorReset)
		os.Exit(1)
	}

	packageName := "Pull Lever"
	if pkgNum == 2 {
		packageName = "Upgrade Race V4"
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"roblox_username": username,
		"package_name":    packageName,
		"price":           price,
	})

	resp, err := doRequest("POST", "/api/admin/orders/create", payload)
	if err != nil {
		fatalf("Gagal menghubungi server: %v (Apakah server menyala?)\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		fatalf("Server Error (%d): %s\n", resp.StatusCode, errData["error"])
	}

	var res struct {
		Message string `json:"message"`
		Order   Order  `json:"order"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fatalf("Gagal mengurai respon server: %v\n", err)
	}

	printBanner()
	fmt.Printf("  %s✓ %s%s\n\n", colorGreen+colorBold, res.Message, colorReset)
	fmt.Printf("  %-20s %s%s%s\n", "Username Roblox:", colorCyan, res.Order.RobloxUsername, colorReset)
	fmt.Printf("  %-20s %s%s%s\n", "Paket Joki:", colorCyan, res.Order.PackageName, colorReset)
	fmt.Printf("  %-20s Rp %s%s%s\n", "Harga:", colorCyan, formatRupiah(res.Order.Price), colorReset)
	fmt.Printf("  %-20s %s%s%s\n", "Status:", colorYellow, res.Order.Status, colorReset)
	fmt.Println()
	fmt.Printf("  ╔══════════════════════════════╗\n")
	fmt.Printf("  ║   PASSCODE PELANGGAN:         ║\n")
	fmt.Printf("  ║   %s%s%-20s%s  ║\n", colorPurple+colorBold, "  ", res.Order.Passcode, colorReset)
	fmt.Printf("  ╚══════════════════════════════╝\n")
	fmt.Println()
	fmt.Printf("  %s→ Kirimkan passcode ini ke pelanggan!%s\n", colorYellow, colorReset)
	fmt.Println()
}

// =====================
// COMMAND: list
// =====================
func cmdList() {
	resp, err := doRequest("GET", "/api/admin/orders", nil)
	if err != nil {
		fatalf("Gagal menghubungi server: %v (Apakah server menyala?)\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		fatalf("Server Error (%d): %s\n", resp.StatusCode, errData["error"])
	}

	var orders []Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		fatalf("Gagal mengurai data server: %v\n", err)
	}

	printBanner()
	if len(orders) == 0 {
		fmt.Printf("  %sℹ  Belum ada pesanan joki saat ini di server.%s\n\n", colorYellow, colorReset)
		return
	}

	fmt.Printf("  %s%sDaftar Pesanan Joki (%d order) - Remote Server%s\n\n", colorBold, colorPurple, len(orders), colorReset)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  %sNO\tPASSCODE\tUSERNAME\tPAKET\tSTATUS\tHARGA\tCATATAN%s\n",
		colorCyan+colorBold, colorReset)
	fmt.Fprintf(w, "  %s──\t────────\t────────\t─────\t──────\t──────\t───────%s\n",
		colorCyan, colorReset)

	for i, o := range orders {
		statusColor := statusToColor(o.Status)
		notesVal := o.Notes
		if notesVal == "" {
			notesVal = "—"
		}
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s%s%s\tRp %s\t%s\n",
			i+1,
			o.Passcode,
			o.RobloxUsername,
			o.PackageName,
			statusColor, o.Status, colorReset,
			formatRupiah(o.Price),
			notesVal,
		)
	}
	w.Flush()
	fmt.Println()
}

// =====================
// COMMAND: status
// =====================
func cmdStatus() {
	if len(os.Args) < 4 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: joki status <passcode> <status> [catatan]")
		fmt.Println()
		fmt.Println("  Pilihan Status: pending | queued | in_progress | completed")
		fmt.Println()
		fmt.Println("  Contoh: joki status BJ-1234 in_progress \"Sedang farming level\"")
		os.Exit(1)
	}

	passcode := strings.TrimSpace(os.Args[2])
	status := strings.TrimSpace(os.Args[3])
	notes := ""
	if len(os.Args) >= 5 {
		notes = strings.Join(os.Args[4:], " ")
	}

	payload, _ := json.Marshal(map[string]string{
		"passcode": passcode,
		"status":   status,
		"notes":    notes,
	})

	resp, err := doRequest("POST", "/api/admin/status", payload)
	if err != nil {
		fatalf("Gagal menghubungi server: %v\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		fatalf("Server Error (%d): %s\n", resp.StatusCode, errData["error"])
	}

	var res struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	printBanner()
	fmt.Printf("  %s✓ %s%s\n\n", colorGreen+colorBold, res.Message, colorReset)
}

// =====================
// COMMAND: delete
// =====================
func cmdDelete() {
	if len(os.Args) < 3 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: joki delete <passcode>")
		os.Exit(1)
	}

	passcode := strings.TrimSpace(os.Args[2])

	fmt.Printf("  %s⚠  Menghapus order remote dengan passcode: %s%s\n", colorYellow, passcode, colorReset)
	fmt.Printf("  Ketik '%sya%s' untuk konfirmasi: ", colorRed, colorReset)

	var confirm string
	fmt.Scanln(&confirm)

	if strings.ToLower(confirm) != "ya" {
		fmt.Printf("  %sℹ  Penghapusan dibatalkan.%s\n\n", colorYellow, colorReset)
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"passcode": passcode,
	})

	resp, err := doRequest("POST", "/api/admin/orders/delete", payload)
	if err != nil {
		fatalf("Gagal menghubungi server: %v\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		fatalf("Server Error (%d): %s\n", resp.StatusCode, errData["error"])
	}

	var res struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	printBanner()
	fmt.Printf("  %s✓ %s%s\n\n", colorGreen, res.Message, colorReset)
}

// =====================
// COMMAND: watch (WebSocket Live Monitoring!)
// =====================
func cmdWatch() {
	if len(os.Args) < 3 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: joki watch <passcode>")
		os.Exit(1)
	}

	passcode := strings.TrimSpace(os.Args[2])

	// Parse server URL untuk menyusun WebSocket URL
	u, err := url.Parse(serverURL)
	if err != nil {
		fatalf("SERVER_URL tidak valid: %v\n", err)
	}

	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/ws?passcode=%s", scheme, u.Host, passcode)

	printBanner()
	fmt.Printf("  %s⚡ Menghubungkan WebSocket live monitoring untuk: %s%s%s...\n", colorPurple, colorCyan, passcode, colorReset)
	fmt.Printf("  URL: %s\n", wsURL)
	fmt.Println("  Tekan Ctrl + C untuk berhenti memantau.")
	fmt.Println()

	// Connect ke WebSocket
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		fatalf("Gagal tersambung ke WebSocket server: %v\n", err)
	}
	defer c.Close()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			fmt.Printf("\n%s[Sistem]%s Koneksi terputus dari server: %v\n", colorRed, colorReset, err)
			break
		}

		var msg struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}

		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		fmt.Printf("  [%s%s%s] ", colorPurple, time.Now().Format("15:04:05"), colorReset)

		if msg.Type == "order_update" {
			var order Order
			if err := json.Unmarshal(msg.Payload, &order); err == nil {
				statusColor := statusToColor(order.Status)
				notesVal := order.Notes
				if notesVal == "" {
					notesVal = "—"
				}
				fmt.Printf("%sLive Update%s ➔ %s (%s%s%s) · Catatan: %s%s%s\n",
					colorGreen, colorReset,
					order.RobloxUsername,
					statusColor, order.Status, colorReset,
					colorYellow, notesVal, colorReset,
				)
			}
		} else if msg.Type == "new_screenshot" {
			var ss struct {
				ID       int    `json:"id"`
				Filename string `json:"filename"`
				URL      string `json:"url"`
			}
			if err := json.Unmarshal(msg.Payload, &ss); err == nil {
				fmt.Printf("%sFoto screenshot live di-upload! 📸%s ➔ Link: %s%s\n",
					colorCyan, colorReset, serverURL, ss.URL,
				)
			}
		}
	}
}

// =====================
// HELPER FUNCTIONS
// =====================
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf(colorRed+"[Error] "+colorReset+format, args...)
	os.Exit(1)
}

func statusToColor(status string) string {
	switch status {
	case "pending":
		return colorYellow
	case "queued":
		return colorCyan
	case "in_progress":
		return colorPurple
	case "completed":
		return colorGreen
	default:
		return colorReset
	}
}

func formatRupiah(amount int) string {
	s := strconv.Itoa(amount)
	var result []byte
	for i, c := range reverseString(s) {
		if i > 0 && i%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return reverseString(string(result))
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func printBanner() {
	fmt.Println()
	fmt.Printf("  %s%s╔══════════════════════════════════════╗%s\n", colorPurple, colorBold, colorReset)
	fmt.Printf("  %s%s║    🎮  JOKI ROBLOX - Remote CLI      ║%s\n", colorPurple, colorBold, colorReset)
	fmt.Printf("  %s%s╚══════════════════════════════════════╝%s\n", colorPurple, colorBold, colorReset)
	fmt.Println()
}

func printHelp() {
	printBanner()
	fmt.Printf("  %sPerintah yang tersedia:%s\n\n", colorBold, colorReset)
	fmt.Printf("  %sadd%s <username> <nomor_paket> <harga>\n", colorCyan, colorReset)
	fmt.Println("       Buat order joki baru di server & dapatkan passcode")
	fmt.Println()
	fmt.Printf("  %slist%s\n", colorCyan, colorReset)
	fmt.Println("       Tampilkan semua pesanan dari remote server")
	fmt.Println()
	fmt.Printf("  %sstatus%s <passcode> <status> [catatan]\n", colorCyan, colorReset)
	fmt.Println("       Ubah status joki di server secara live")
	fmt.Println()
	fmt.Printf("  %sdelete%s <passcode>\n", colorCyan, colorReset)
	fmt.Println("       Hapus pesanan di server secara remote")
	fmt.Println()
	fmt.Printf("  %swatch%s <passcode>\n", colorCyan, colorReset)
	fmt.Println("       Pantau kemajuan joki secara live (WebSocket) di terminal")
	fmt.Println()
}

// setupWorkDir auto-detects the project root (where go.mod is located)
// and shifts the working directory to it so relative paths (.env) are always valid.
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
					// Silent shift
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

// doRequest membuat request HTTP dengan autentikasi X-Admin-Password dan Cookie token admin.
// Jika terjadi error 401 Unauthorized, CLI akan meminta input password admin dan mencoba kembali.
func doRequest(method, path string, bodyData []byte) (*http.Response, error) {
	for attempt := 1; attempt <= 2; attempt++ {
		reqURL := serverURL + path
		var bodyReader io.Reader
		if bodyData != nil {
			bodyReader = bytes.NewReader(bodyData)
		}

		req, err := http.NewRequest(method, reqURL, bodyReader)
		if err != nil {
			return nil, err
		}

		if bodyData != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		// Kirim kedua metode autentikasi agar kompatibel dengan server lama maupun baru
		req.Header.Set("X-Admin-Password", adminPassword)
		adminToken := fmt.Sprintf("admin-%x", []byte(adminPassword))
		req.Header.Set("Cookie", fmt.Sprintf("admin_token=%s", adminToken))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		// Jika 401 Unauthorized dan ini percobaan pertama, minta password dari user
		if resp.StatusCode == http.StatusUnauthorized && attempt == 1 {
			resp.Body.Close()
			fmt.Printf("\n%s[Akses Ditolak]%s Server di %s memerlukan password admin yang valid.\n", colorYellow, colorReset, serverURL)
			fmt.Print("  Masukkan Password Admin: ")
			var inputPass string
			_, scanErr := fmt.Scanln(&inputPass)
			if scanErr != nil || inputPass == "" {
				return nil, fmt.Errorf("password tidak boleh kosong")
			}

			// Simpan password ke variabel memori dan ulangi request
			adminPassword = strings.TrimSpace(inputPass)
			continue
		}

		// Jika berhasil di percobaan kedua, simpan password baru ke .env
		if resp.StatusCode == http.StatusOK && attempt == 2 {
			savePasswordToEnv(adminPassword)
		}

		return resp, nil
	}
	return nil, fmt.Errorf("akses ditolak")
}

// savePasswordToEnv menulis atau memperbarui password admin di file .env lokal laptop
func savePasswordToEnv(pass string) {
	envPath := ".env"

	// Baca file .env lama jika ada
	content, err := os.ReadFile(envPath)
	if err != nil {
		// File belum ada, buat baru
		newContent := fmt.Sprintf("SERVER_URL=%s\nADMIN_PASSWORD=%s\n", serverURL, pass)
		_ = os.WriteFile(envPath, []byte(newContent), 0644)
		fmt.Printf("  %s✓%s Password admin berhasil disimpan ke %s\n\n", colorGreen, colorReset, envPath)
		return
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ADMIN_PASSWORD=") {
			lines[i] = fmt.Sprintf("ADMIN_PASSWORD=%s", pass)
			found = true
			break
		}
	}

	if !found {
		lines = append(lines, fmt.Sprintf("ADMIN_PASSWORD=%s", pass))
	}

	newContent := strings.Join(lines, "\n")
	_ = os.WriteFile(envPath, []byte(newContent), 0644)
	fmt.Printf("  %s✓%s Password admin berhasil disimpan di %s\n\n", colorGreen, colorReset, envPath)
}
