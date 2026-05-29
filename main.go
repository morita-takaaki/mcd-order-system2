package main

import (
	"context"
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

	// logsフォルダが存在しない場合は自動作成
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("ログフォルダの作成に失敗しました: %v", err)
	}

	// 追記モードでログファイルを開く
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("ログファイルのオープンに失敗しました: %v", err)
	}
	defer logFile.Close()

	// ログの出力先を標準出力とファイルの両方に設定
	log.SetOutput(os.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// 2. データベースの初期化
	initDB()
	defer db.Close()

	// 3. ルーティングの設定 (Go 1.22+ の http.NewServeMux)
	mux := http.NewServeMux()

	// 注文関連API
	mux.HandleFunc("POST /api/orders", handleCreateOrder)
	mux.HandleFunc("GET /api/orders", handleGetOrders)
	mux.HandleFunc("GET /api/orders/{orderNo}", handleGetOrderDetail)
	mux.HandleFunc("PUT /api/orders/{orderNo}/status", handleUpdateOrderStatus)

	// フロント掲示板用API
	mux.HandleFunc("GET /api/board", handleGetBoard)
	mux.HandleFunc("PUT /api/board", handleUpdateBoard)

	// 厨房用API
	mux.HandleFunc("GET /api/kitchen", handleGetKitchen)
	mux.HandleFunc("PUT /api/kitchen", handleUpdateKitchen)

	// 4. CORS対応ミドルウェアの適用
	loggingAndCORSMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CORSヘッダーの付与
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			// OPTIONSメソッド（プリフライト）の場合は200 OKで早期返却
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			// リクエストログの出力
			log.Printf("[REQ] %s %s", r.Method, r.URL.Path)

			// 本来のハンドラー処理を実行
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