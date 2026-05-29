package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "order.db")
	if err != nil {
		log.Fatalf("データベースのオープンに失敗しました: %v", err)
	}

	// SQLiteの同時書き込み対策
	db.SetMaxOpenConns(1)

	if err = db.Ping(); err != nil {
		log.Fatalf("データベースへの接続確認に失敗しました: %v", err)
	}

	// テーブル自動作成
	createTableSQL := `
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

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("tableの作成に失敗しました: %v", err)
	}
}