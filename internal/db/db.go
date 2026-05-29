package db

import (
	"database/sql"
	"fmt"
	"joki_roblox/internal/models"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

// DB adalah instance database global
type DB struct {
	conn *sql.DB
}

// New membuat koneksi baru ke SQLite dan melakukan migrasi tabel secara otomatis
func New(dbPath string) (*DB, error) {
	// Pastikan direktori parent dari file db sudah ada
	dir := filepath.Dir(dbPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("gagal membuat direktori db: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka database: %w", err)
	}

	// Aktifkan WAL mode agar performa baca/tulis lebih cepat
	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("gagal set WAL mode: %w", err)
	}
	// Aktifkan foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("gagal aktifkan foreign keys: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("gagal migrasi database: %w", err)
	}

	log.Printf("[DB] Database siap di: %s", dbPath)
	return db, nil
}

// migrate membuat tabel jika belum ada
func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS orders (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			passcode        TEXT    NOT NULL UNIQUE,
			roblox_username TEXT    NOT NULL,
			package_name    TEXT    NOT NULL,
			status          TEXT    NOT NULL DEFAULT 'pending',
			price           INTEGER NOT NULL DEFAULT 0,
			notes           TEXT    NOT NULL DEFAULT '',
			created_at      DATETIME NOT NULL,
			updated_at      DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS screenshots (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id   INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			filename   TEXT    NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS push_subscriptions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id   INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			endpoint   TEXT    NOT NULL UNIQUE,
			p256dh     TEXT    NOT NULL,
			auth       TEXT    NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_passcode ON orders(passcode)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_username ON orders(roblox_username)`,
		`CREATE INDEX IF NOT EXISTS idx_screenshots_order_id ON screenshots(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_push_subscriptions_order ON push_subscriptions(order_id)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("gagal eksekusi migrasi: %w\nQuery: %s", err, q)
		}
	}
	return nil
}

// Close menutup koneksi database
func (db *DB) Close() error {
	return db.conn.Close()
}

// =====================
// CRUD ORDER
// =====================

// CreateOrder membuat pesanan baru dan menyimpannya ke database
func (db *DB) CreateOrder(order *models.Order) error {
	now := time.Now()
	order.CreatedAt = now
	order.UpdatedAt = now

	result, err := db.conn.Exec(
		`INSERT INTO orders (passcode, roblox_username, package_name, status, price, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		order.Passcode,
		order.RobloxUsername,
		string(order.PackageName),
		string(order.Status),
		order.Price,
		order.Notes,
		order.CreatedAt.Format(time.RFC3339),
		order.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("gagal membuat order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	order.ID = int(id)
	return nil
}

// GetOrderByPasscode mengambil detail order beserta screenshot-nya berdasarkan passcode
func (db *DB) GetOrderByPasscode(passcode string) (*models.OrderDetail, error) {
	row := db.conn.QueryRow(
		`SELECT id, passcode, roblox_username, package_name, status, price, notes, created_at, updated_at
		 FROM orders WHERE passcode = ?`, passcode,
	)

	detail := &models.OrderDetail{}
	var createdAt, updatedAt string
	err := row.Scan(
		&detail.ID, &detail.Passcode, &detail.RobloxUsername,
		&detail.PackageName, &detail.Status, &detail.Price, &detail.Notes,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Tidak ditemukan
	}
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil order: %w", err)
	}

	detail.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	detail.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	// Ambil screenshots terkait
	screenshots, err := db.GetScreenshotsByOrderID(detail.ID)
	if err != nil {
		return nil, err
	}
	detail.Screenshots = screenshots
	if detail.Screenshots == nil {
		detail.Screenshots = []models.Screenshot{}
	}

	return detail, nil
}

// GetOrderByUsername mengambil order terbaru berdasarkan username Roblox
func (db *DB) GetOrderByUsername(username string) (*models.OrderDetail, error) {
	row := db.conn.QueryRow(
		`SELECT id, passcode, roblox_username, package_name, status, price, notes, created_at, updated_at
		 FROM orders WHERE roblox_username = ? ORDER BY created_at DESC LIMIT 1`, username,
	)

	detail := &models.OrderDetail{}
	var createdAt, updatedAt string
	err := row.Scan(
		&detail.ID, &detail.Passcode, &detail.RobloxUsername,
		&detail.PackageName, &detail.Status, &detail.Price, &detail.Notes,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil order by username: %w", err)
	}

	detail.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	detail.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	screenshots, err := db.GetScreenshotsByOrderID(detail.ID)
	if err != nil {
		return nil, err
	}
	detail.Screenshots = screenshots
	if detail.Screenshots == nil {
		detail.Screenshots = []models.Screenshot{}
	}

	return detail, nil
}

// ListOrders mengambil semua order (tanpa screenshot) untuk ditampilkan di CLI/Admin
func (db *DB) ListOrders() ([]models.Order, error) {
	rows, err := db.conn.Query(
		`SELECT id, passcode, roblox_username, package_name, status, price, notes, created_at, updated_at
		 FROM orders ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal list orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		var createdAt, updatedAt string
		if err := rows.Scan(
			&o.ID, &o.Passcode, &o.RobloxUsername,
			&o.PackageName, &o.Status, &o.Price, &o.Notes,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		o.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		orders = append(orders, o)
	}
	return orders, nil
}

// UpdateOrderStatus mengubah status dan notes pada sebuah order berdasarkan passcode
func (db *DB) UpdateOrderStatus(passcode string, status models.OrderStatus, notes string) error {
	now := time.Now().Format(time.RFC3339)
	result, err := db.conn.Exec(
		`UPDATE orders SET status = ?, notes = ?, updated_at = ? WHERE passcode = ?`,
		string(status), notes, now, passcode,
	)
	if err != nil {
		return fmt.Errorf("gagal update status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("order dengan passcode '%s' tidak ditemukan", passcode)
	}
	return nil
}

// DeleteOrder menghapus order beserta semua screenshot-nya berdasarkan passcode
func (db *DB) DeleteOrder(passcode string) error {
	result, err := db.conn.Exec(`DELETE FROM orders WHERE passcode = ?`, passcode)
	if err != nil {
		return fmt.Errorf("gagal menghapus order: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("order dengan passcode '%s' tidak ditemukan", passcode)
	}
	return nil
}

// =====================
// CRUD SCREENSHOT
// =====================

// AddScreenshot menyimpan data screenshot baru ke database
func (db *DB) AddScreenshot(screenshot *models.Screenshot) error {
	now := time.Now()
	screenshot.CreatedAt = now

	result, err := db.conn.Exec(
		`INSERT INTO screenshots (order_id, filename, created_at) VALUES (?, ?, ?)`,
		screenshot.OrderID,
		screenshot.Filename,
		screenshot.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("gagal menyimpan screenshot: %w", err)
	}
	id, _ := result.LastInsertId()
	screenshot.ID = int(id)
	return nil
}

// GetScreenshotsByOrderID mengambil semua screenshot berdasarkan ID order
func (db *DB) GetScreenshotsByOrderID(orderID int) ([]models.Screenshot, error) {
	rows, err := db.conn.Query(
		`SELECT id, order_id, filename, created_at FROM screenshots WHERE order_id = ? ORDER BY created_at ASC`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil screenshots: %w", err)
	}
	defer rows.Close()

	var screenshots []models.Screenshot
	for rows.Next() {
		var s models.Screenshot
		var createdAt string
		if err := rows.Scan(&s.ID, &s.OrderID, &s.Filename, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		s.URL = "/uploads/" + s.Filename
		screenshots = append(screenshots, s)
	}
	return screenshots, nil
}

// GetOrderIDByPasscode adalah helper untuk mengambil ID numerik order berdasarkan passcode
func (db *DB) GetOrderIDByPasscode(passcode string) (int, error) {
	var id int
	err := db.conn.QueryRow(`SELECT id FROM orders WHERE passcode = ?`, passcode).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("order dengan passcode '%s' tidak ditemukan", passcode)
	}
	return id, err
}

// =====================
// PUSH SUBSCRIPTION
// =====================

// AddSubscription menyimpan detail langganan push baru ke database,
// atau memperbaruinya jika endpoint yang sama sudah terdaftar
func (db *DB) AddSubscription(orderID int, endpoint, p256dh, auth string) error {
	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT OR REPLACE INTO push_subscriptions (order_id, endpoint, p256dh, auth, created_at) VALUES (?, ?, ?, ?, ?)`,
		orderID,
		endpoint,
		p256dh,
		auth,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("gagal menyimpan push subscription: %w", err)
	}
	return nil
}

// GetSubscriptionsByOrderID mengambil semua push subscription berdasarkan ID order
func (db *DB) GetSubscriptionsByOrderID(orderID int) ([]models.PushSubscription, error) {
	rows, err := db.conn.Query(
		`SELECT id, order_id, endpoint, p256dh, auth, created_at FROM push_subscriptions WHERE order_id = ?`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil push subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []models.PushSubscription
	for rows.Next() {
		var sub models.PushSubscription
		var createdAt string
		if err := rows.Scan(&sub.ID, &sub.OrderID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &createdAt); err != nil {
			return nil, err
		}
		sub.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		subs = append(subs, sub)
	}
	return subs, nil
}

// DeleteSubscriptionByEndpoint menghapus token push yang sudah tidak valid / kedaluwarsa
func (db *DB) DeleteSubscriptionByEndpoint(endpoint string) error {
	_, err := db.conn.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}
