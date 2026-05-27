package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// 1. ログフォルダとファイルの自動作成
	logDir := "logs"
	logPath := filepath.Join(logDir, "order.log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("ログフォルダの作成に失敗しました: %v", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("ログファイルのオープンに失敗しました: %v", err)
	}
	defer logFile.Close()

	// 標準ロガーの出力先を設定ファイルに変更
	log.SetOutput(logFile)

	// 2. データベースの初期化
	initDB()
	defer db.Close()

	// 3. ルーティングの設定 (標準の http.ServeMux を使用)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/orders", handleOrders)          // POST(登録), GET(一覧・状態別)
	mux.HandleFunc("/api/orders/", handleOrderWithParam) // GET(詳細), PUT(状態更新)

	// 4. CORSミドルウェアの適用
	loggingAndCORSHandler := corsMiddleware(mux)

	// 5. サーバー起動表示と開始
	fmt.Println("サーバー起動: http://localhost:8080")
	if err := http.ListenAndServe(":8080", loggingAndCORSHandler); err != nil {
		log.Fatalf("サーバーの起動に失敗しました: %v", err)
	}
}

// CORSおよびOPTIONS（プリフライト）に対応するミドルウェア
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { // http.Context から修正
		// すべてのオリジンからのリクエストを許可
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// OPTIONSメソッド（プリフライトリクエスト）の場合はここで200を返して終了
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}