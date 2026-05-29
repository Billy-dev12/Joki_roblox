package main

import (
	"fmt"
	"joki_roblox/internal/db"
	"joki_roblox/internal/models"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

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

var database *db.DB

func main() {
	setupWorkDir()

	// Load konfigurasi
	_ = godotenv.Load()
	dbPath := getEnv("DB_PATH", "./joki.db")

	var err error
	database, err = db.New(dbPath)
	if err != nil {
		fatalf("Gagal koneksi ke database: %v\n", err)
	}
	defer database.Close()

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

// cmdAdd menambahkan pesanan joki baru dan menampilkan passcode yang dibuat secara otomatis
// Usage: cli add <roblox_username> <package_number> <price>
// Contoh: cli add NarutoUzumaki 1 50000
func cmdAdd() {
	if len(os.Args) < 5 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: cli add <roblox_username> <nomor_paket> <harga>")
		fmt.Println()
		fmt.Println("  Pilihan Paket:")
		for i, pkg := range models.AllPackages {
			fmt.Printf("    %d. %s\n", i+1, pkg)
		}
		fmt.Println()
		fmt.Println("  Contoh: cli add NarutoUzumaki 1 50000")
		os.Exit(1)
	}

	username := strings.TrimSpace(os.Args[2])
	pkgNum, err := strconv.Atoi(os.Args[3])
	if err != nil || pkgNum < 1 || pkgNum > len(models.AllPackages) {
		fmt.Printf("%s[Error]%s Nomor paket tidak valid. Pilih antara 1 - %d.\n",
			colorRed, colorReset, len(models.AllPackages))
		os.Exit(1)
	}

	price, err := strconv.Atoi(os.Args[4])
	if err != nil || price <= 0 {
		fmt.Printf("%s[Error]%s Harga harus berupa angka positif (dalam Rupiah).\n", colorRed, colorReset)
		os.Exit(1)
	}

	selectedPackage := models.AllPackages[pkgNum-1]
	passcode := generatePasscode()

	order := &models.Order{
		Passcode:       passcode,
		RobloxUsername: username,
		PackageName:    selectedPackage,
		Status:         models.StatusPending,
		Price:          price,
	}

	if err := database.CreateOrder(order); err != nil {
		fatalf("Gagal membuat order: %v\n", err)
	}

	printBanner()
	fmt.Printf("  %s✓ Order Berhasil Dibuat!%s\n\n", colorGreen+colorBold, colorReset)
	fmt.Printf("  %-20s %s%s%s\n", "Username Roblox:", colorCyan, username, colorReset)
	fmt.Printf("  %-20s %s%s%s\n", "Paket Joki:", colorCyan, selectedPackage, colorReset)
	fmt.Printf("  %-20s Rp %s%s%s\n", "Harga:", colorCyan, formatRupiah(price), colorReset)
	fmt.Printf("  %-20s %s%s%s\n", "Status:", colorYellow, "Pending", colorReset)
	fmt.Println()
	fmt.Printf("  ╔══════════════════════════════╗\n")
	fmt.Printf("  ║   PASSCODE PELANGGAN:         ║\n")
	fmt.Printf("  ║   %s%s%-20s%s  ║\n", colorPurple+colorBold, "  ", passcode, colorReset)
	fmt.Printf("  ╚══════════════════════════════╝\n")
	fmt.Println()
	fmt.Printf("  %s→ Kirimkan passcode ini ke pelanggan!%s\n", colorYellow, colorReset)
	fmt.Println()
}

// =====================
// COMMAND: list
// =====================

// cmdList menampilkan semua order dalam format tabel
// Usage: cli list
func cmdList() {
	orders, err := database.ListOrders()
	if err != nil {
		fatalf("Gagal mengambil data order: %v\n", err)
	}

	printBanner()

	if len(orders) == 0 {
		fmt.Printf("  %sℹ  Belum ada pesanan joki saat ini.%s\n\n", colorYellow, colorReset)
		return
	}

	fmt.Printf("  %s%sDaftar Pesanan Joki (%d order)%s\n\n", colorBold, colorPurple, len(orders), colorReset)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  %sNO\tPASSCODE\tUSERNAME\tPAKET\tSTATUS\tHARGA\tTANGGAL%s\n",
		colorCyan+colorBold, colorReset)
	fmt.Fprintf(w, "  %s──\t────────\t────────\t─────\t──────\t──────\t───────%s\n",
		colorCyan, colorReset)

	for i, o := range orders {
		statusColor := statusToColor(o.Status)
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s%s%s\tRp %s\t%s\n",
			i+1,
			o.Passcode,
			o.RobloxUsername,
			o.PackageName,
			statusColor, o.Status, colorReset,
			formatRupiah(o.Price),
			o.CreatedAt.Format("02/01/06 15:04"),
		)
	}
	w.Flush()
	fmt.Println()
}

// =====================
// COMMAND: status
// =====================

// cmdStatus mengubah status sebuah order berdasarkan passcode
// Usage: cli status <passcode> <status> [notes]
// Contoh: cli status BJ-1234 in_progress "Sedang farming level"
func cmdStatus() {
	if len(os.Args) < 4 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: cli status <passcode> <status> [catatan]")
		fmt.Println()
		fmt.Println("  Pilihan Status:")
		fmt.Println("    pending     → Menunggu diproses")
		fmt.Println("    queued      → Dalam antrean")
		fmt.Println("    in_progress → Sedang dikerjakan")
		fmt.Println("    completed   → Selesai")
		fmt.Println()
		fmt.Println("  Contoh: cli status BJ-1234 in_progress")
		os.Exit(1)
	}

	passcode := strings.TrimSpace(os.Args[2])
	newStatus := models.OrderStatus(strings.TrimSpace(os.Args[3]))
	notes := ""
	if len(os.Args) >= 5 {
		notes = strings.Join(os.Args[4:], " ")
	}

	// Validasi status
	validStatuses := map[models.OrderStatus]bool{
		models.StatusPending:    true,
		models.StatusQueued:     true,
		models.StatusInProgress: true,
		models.StatusCompleted:  true,
	}
	if !validStatuses[newStatus] {
		fmt.Printf("%s[Error]%s Status '%s' tidak valid.\n", colorRed, colorReset, newStatus)
		fmt.Println("  Status yang valid: pending, queued, in_progress, completed")
		os.Exit(1)
	}

	if err := database.UpdateOrderStatus(passcode, newStatus, notes); err != nil {
		fatalf("[Error] %v\n", err)
	}

	printBanner()
	fmt.Printf("  %s✓ Status Berhasil Diubah!%s\n\n", colorGreen+colorBold, colorReset)
	fmt.Printf("  Passcode  : %s%s%s\n", colorCyan, passcode, colorReset)
	fmt.Printf("  Status    : %s%s%s\n", statusToColor(newStatus), newStatus, colorReset)
	if notes != "" {
		fmt.Printf("  Catatan   : %s\n", notes)
	}
	fmt.Println()
	fmt.Printf("  %s→ Update real-time akan terkirim ke browser pelanggan via WebSocket!%s\n", colorYellow, colorReset)
	fmt.Println()
}

// =====================
// COMMAND: delete
// =====================

// cmdDelete menghapus order berdasarkan passcode
// Usage: cli delete <passcode>
func cmdDelete() {
	if len(os.Args) < 3 {
		fmt.Printf("%s[Error]%s Kurang argumen!\n", colorRed, colorReset)
		fmt.Println("  Penggunaan: cli delete <passcode>")
		os.Exit(1)
	}

	passcode := strings.TrimSpace(os.Args[2])

	fmt.Printf("  %s⚠  Menghapus order dengan passcode: %s%s\n", colorYellow, passcode, colorReset)
	fmt.Printf("  Ketik '%sya%s' untuk konfirmasi: ", colorRed, colorReset)

	var confirm string
	fmt.Scanln(&confirm)

	if strings.ToLower(confirm) != "ya" {
		fmt.Printf("  %sℹ  Penghapusan dibatalkan.%s\n\n", colorYellow, colorReset)
		return
	}

	if err := database.DeleteOrder(passcode); err != nil {
		fatalf("[Error] %v\n", err)
	}

	fmt.Printf("  %s✓ Order %s berhasil dihapus!%s\n\n", colorGreen, passcode, colorReset)
}

// =====================
// HELPER FUNCTIONS
// =====================

// generatePasscode membuat kode unik acak dalam format BJ-XXXX
func generatePasscode() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Hindari 0,O,I,1 yang mudah tertukar
	code := make([]byte, 4)
	for i := range code {
		code[i] = chars[rng.Intn(len(chars))]
	}
	return fmt.Sprintf("BJ-%s", string(code))
}

// formatRupiah memformat angka ke format Rupiah (1.500.000)
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

// statusToColor mengembalikan kode warna ANSI berdasarkan status
func statusToColor(status models.OrderStatus) string {
	switch status {
	case models.StatusPending:
		return colorYellow
	case models.StatusQueued:
		return colorCyan
	case models.StatusInProgress:
		return colorPurple
	case models.StatusCompleted:
		return colorGreen
	default:
		return colorReset
	}
}

// printBanner mencetak banner aplikasi
func printBanner() {
	fmt.Println()
	fmt.Printf("  %s%s╔══════════════════════════════════════╗%s\n", colorPurple, colorBold, colorReset)
	fmt.Printf("  %s%s║    🎮  JOKI ROBLOX - Admin CLI       ║%s\n", colorPurple, colorBold, colorReset)
	fmt.Printf("  %s%s╚══════════════════════════════════════╝%s\n", colorPurple, colorBold, colorReset)
	fmt.Println()
}

// printHelp mencetak panduan penggunaan CLI
func printHelp() {
	printBanner()
	fmt.Printf("  %sPerintah yang tersedia:%s\n\n", colorBold, colorReset)
	fmt.Printf("  %sadd%s <username> <nomor_paket> <harga>\n", colorCyan, colorReset)
	fmt.Println("       Buat order joki baru & generate passcode pelanggan")
	fmt.Println("       Contoh: cli add NarutoUzumaki 1 50000")
	fmt.Println()
	fmt.Printf("  %slist%s\n", colorCyan, colorReset)
	fmt.Println("       Tampilkan semua pesanan joki")
	fmt.Println()
	fmt.Printf("  %sstatus%s <passcode> <status> [catatan]\n", colorCyan, colorReset)
	fmt.Println("       Ubah status pengerjaan joki")
	fmt.Println("       Status: pending | queued | in_progress | completed")
	fmt.Println("       Contoh: cli status BJ-AB12 in_progress")
	fmt.Println()
	fmt.Printf("  %sdelete%s <passcode>\n", colorCyan, colorReset)
	fmt.Println("       Hapus sebuah pesanan joki")
	fmt.Println()
	fmt.Printf("  Paket tersedia:\n")
	for i, pkg := range models.AllPackages {
		fmt.Printf("    %d. %s\n", i+1, pkg)
	}
	fmt.Println()
}

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
					// Silent shift for CLI to keep stdout clean
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
