package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// 1. ログの設定
	logDir := "logs"
	logPath := filepath.Join(logDir, "order.log")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("ログフォルダの作成に失敗しました: %v", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("ログファイルのオープンに失敗しました: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(os.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// 2. データベースの初期化
	initDB()
	defer db.Close()

	// 3. ルーティングの設定 (既存のHandler構造を維持して拡張)
	mux := http.NewServeMux()

	// 注文関連API
	mux.HandleFunc("/api/orders", OrdersHandler(db))
	mux.HandleFunc("/api/orders/", OrderDetailHandler(db)) // パスパラメータ対応用

	// フロント掲示板用API (新規追加・統合)
	mux.HandleFunc("/api/board", BoardHandler(db))

	// 厨房用API (新規追加・統合)
	mux.HandleFunc("/api/kitchen", KitchenHandler(db))

	// 4. CORS対応ミドルウェア
	loggingAndCORSMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			log.Printf("[REQ] %s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}

	// 5. サーバーの起動
	addr := "0.0.0.0:8080"
	fmt.Printf("サーバー起動: http://localhost:8080\n")
	if err := http.ListenAndServe(addr, loggingAndCORSMiddleware(mux)); err != nil {
		log.Fatalf("サーバーの起動に失敗しました: %v", err)
	}
}