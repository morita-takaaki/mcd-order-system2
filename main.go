package main

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// ログファイルと標準出力を同時に出力する設定 (Go 1.25互換)
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("ログファイルのオープンに失敗しました: %v", err)
	}
	defer logFile.Close()

	// io.MultiWriter を使用して標準出力とファイルの両方にログを書き出す
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)

	// データベース接続
	db, err := sql.Open("sqlite3", "./order.db")
	if err != nil {
		log.Fatalf("データベース接続エラー: %v", err)
	}
	defer db.Close()

	// ルーティング設定
	mux := http.NewServeMux()
	
	// 注文管理 API
	mux.HandleFunc("/api/orders", OrdersHandler(db))
	mux.HandleFunc("/api/orders/", OrderDetailHandler(db))
	
	// フロント掲示板 API
	mux.HandleFunc("/api/board", BoardHandler(db))
	mux.HandleFunc("/api/board/", BoardHandler(db))
	
	// 厨房画面 API
	mux.HandleFunc("/api/kitchen", KitchenHandler(db))
	mux.HandleFunc("/api/kitchen/", KitchenHandler(db))

	log.Println("サーバー起動: http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("サーバー強制終了: %v", err)
	}
}