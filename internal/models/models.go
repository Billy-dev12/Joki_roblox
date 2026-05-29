package models

import "time"

// PackageName adalah enum untuk nama paket joki Blox Fruit yang tersedia
type PackageName string

const (
	PackagePullLever     PackageName = "Pull Lever"
	PackageUpgradeRaceV4 PackageName = "Upgrade Race V4"
)

// OrderStatus adalah enum untuk status pengerjaan joki
type OrderStatus string

const (
	StatusPending    OrderStatus = "pending"
	StatusQueued     OrderStatus = "queued"
	StatusInProgress OrderStatus = "in_progress"
	StatusCompleted  OrderStatus = "completed"
)

// AllPackages adalah daftar seluruh paket yang tersedia (untuk validasi & CLI)
var AllPackages = []PackageName{
	PackagePullLever,
	PackageUpgradeRaceV4,
}

// AllStatuses adalah daftar seluruh status yang valid
var AllStatuses = []OrderStatus{
	StatusPending,
	StatusQueued,
	StatusInProgress,
	StatusCompleted,
}

// Order merepresentasikan satu pesanan joki dari seorang pelanggan
type Order struct {
	ID             int         `json:"id"`
	Passcode       string      `json:"passcode"`        // Kode unik sementara untuk pelanggan (contoh: BJ-9021)
	RobloxUsername string      `json:"roblox_username"` // Username Roblox yang akan di-joki
	PackageName    PackageName `json:"package_name"`    // Nama paket joki yang dipesan
	Status         OrderStatus `json:"status"`          // Status pengerjaan saat ini
	Price          int         `json:"price"`           // Harga dalam Rupiah (IDR)
	Notes          string      `json:"notes"`           // Catatan tambahan dari admin (opsional)
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Screenshot merepresentasikan satu gambar bukti pengerjaan joki
type Screenshot struct {
	ID        int       `json:"id"`
	OrderID   int       `json:"order_id"`
	Filename  string    `json:"filename"`   // Nama file gambar yang tersimpan di /public/uploads/
	URL       string    `json:"url"`        // URL publik untuk mengakses gambar
	CreatedAt time.Time `json:"created_at"`
}

// OrderDetail adalah gabungan Order beserta daftar screenshot-nya,
// dikirimkan ke pelanggan via API maupun WebSocket
type OrderDetail struct {
	Order
	Screenshots []Screenshot `json:"screenshots"`
}

// WSMessage adalah format pesan yang dikirim melalui WebSocket ke browser pelanggan
type WSMessage struct {
	Type    string      `json:"type"`    // "order_update", "new_screenshot", "error"
	Payload interface{} `json:"payload"` // Isi data (OrderDetail, Screenshot, atau string error)
}

// AdminLoginRequest adalah body untuk endpoint POST /api/admin/login
type AdminLoginRequest struct {
	Password string `json:"password"`
}

// UpdateStatusRequest adalah body untuk endpoint POST /api/admin/status
type UpdateStatusRequest struct {
	Passcode string      `json:"passcode"`
	Status   OrderStatus `json:"status"`
	Notes    string      `json:"notes"`
}

// PushSubscription merepresentasikan detail langganan push dari browser
type PushSubscription struct {
	ID        int       `json:"id"`
	OrderID   int       `json:"order_id"`
	Endpoint  string    `json:"endpoint"`
	P256dh    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	CreatedAt time.Time `json:"created_at"`
}

// SaveSubscriptionRequest adalah payload POST /api/save-subscription
type SaveSubscriptionRequest struct {
	Passcode string `json:"passcode"`
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}
