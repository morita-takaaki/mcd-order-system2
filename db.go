package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

// データベースの初期化とテーブル自動作成
func initDB() {
	var err error
	// order.db が無い場合は自動作成される
	db, err = sql.Open("sqlite3", "./order.db")
	if err != nil {
		log.Fatalf("DB接続エラー: %v", err)
	}

	// テーブル作成SQLの実行
	query := `
	CREATE TABLE IF NOT EXISTS order_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_no TEXT NOT NULL,
		terminal_no TEXT NOT NULL,
		order_status TEXT NOT NULL,
		item_no INTEGER NOT NULL,
		menu_name TEXT NOT NULL,
		unit_price INTEGER NOT NULL,
		quantity INTEGER NOT NULL,
		subtotal INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(query); err != nil {
		log.Fatalf("テーブル作成エラー: %v", err)
	}
}

// 当日の最大連番を取得し、新しい注文番号（MMDD-NNN）を発行する
func generateOrderNo() (string, error) {
	now := time.Now()
	dateStr := now.Format("0102") // MMDD形式
	likePattern := dateStr + "-%"

	// 当日の最大のorder_noを取得
	var maxOrderNo sql.NullString
	query := "SELECT MAX(order_no) FROM order_items WHERE order_no LIKE ?"
	err := db.QueryRow(query, likePattern).Scan(&maxOrderNo)
	if err != nil {
		return "", err
	}

	nextSeq := 1
	if maxOrderNo.Valid && maxOrderNo.String != "" {
		// 取得した文字列（例: "0424-002"）の末尾3桁を数値化
		var m0102 string
		var seq int
		_, err := fmt.Sscanf(maxOrderNo.String, "%4s-%3d", &m0102, &seq)
		if err == nil {
			nextSeq = seq + 1
		}
	}

	// MMDD-NNN 形式に整形
	return fmt.Sprintf("%s-%03d", dateStr, nextSeq), nil
}
