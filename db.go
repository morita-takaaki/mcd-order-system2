package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

// initDB はデータベースの初期化と接続を行います
func initDB() {
	var err error
	// order.db ファイルを開く（存在しない場合は自動作成されます）
	db, err = sql.Open("sqlite3", "order.db")
	if err != nil {
		log.Fatalf("データベースのオープンに失敗しました: %v", err)
	}

	// SQLiteの同時書き込み（ロック競合）対策として最大接続数を1に制限
	db.SetMaxOpenConns(1)

	// 接続確認
	if err = db.Ping(); err != nil {
		log.Fatalf("データベースへの接続確認に失敗しました: %v", err)
	}

	// order_items テーブルが存在しない場合は自動作成するSQL
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