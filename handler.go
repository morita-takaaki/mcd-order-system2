package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// --- 構造体定義 ---
type OrderItemReq struct {
	MenuName  string `json:"menuName"`
	UnitPrice int    `json:"unitPrice"`
	Quantity  int    `json:"quantity"`
}

type OrderCreateReq struct {
	TerminalNo  string         `json:"terminalNo"`
	MessageType string         `json:"messageType"`
	TotalAmount int            `json:"totalAmount"`
	Items       []OrderItemReq `json:"items"`
}

type KitchenItem struct {
	MenuName string `json:"menuName"`
	Quantity int    `json:"quantity"`
}

type KitchenOrder struct {
	OrderNo string        `json:"orderNo"`
	Items   []KitchenItem `json:"items"`
}

// --- 共通ユーティリティ ---
func writeLog(title string, data interface{}) {
	bytes, _ := json.Marshal(data)
	log.Printf("[%s] %s", title, string(bytes))
}

func generateOrderNo(database *sql.DB) (string, error) {
	today := time.Now().Format("0102")
	var maxOrderNo sql.NullString

	err := database.QueryRow("SELECT MAX(order_no) FROM order_items WHERE order_no LIKE ?", today+"-%").Scan(&maxOrderNo)
	if err != nil {
		return "", err
	}

	nextNum := 1
	if maxOrderNo.Valid {
		var lastNum int
		_, err := fmt.Sscanf(maxOrderNo.String, today+"-%d", &lastNum)
		if err == nil {
			nextNum = lastNum + 1
		}
	}
	return fmt.Sprintf("%s-%03d", today, nextNum), nil
}

// --- ハンドラー実装 ---

// 1. 注文登録 & 一覧取得 ハンドラー
func OrdersHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// GET /api/orders (一覧取得 & 状態別一覧取得)
		if r.Method == http.MethodGet {
			statusFilter := r.URL.Query().Get("status")
			var rows *sql.Rows
			var err error

			if statusFilter != "" {
				query := `SELECT order_no, terminal_no, order_status, SUM(subtotal) FROM order_items WHERE order_status = ? GROUP BY order_no ORDER BY created_at ASC`
				rows, err = database.Query(query, statusFilter)
			} else {
				query := `SELECT order_no, terminal_no, order_status, SUM(subtotal) FROM order_items GROUP BY order_no ORDER BY created_at ASC`
				rows, err = database.Query(query)
			}

			if err != nil {
				http.Error(w, "データ取得失敗", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			results := []map[string]interface{}{}
			for rows.Next() {
				var orderNo, terminalNo, orderStatus string
				var totalAmount int
				rows.Scan(&orderNo, &terminalNo, &orderStatus, &totalAmount)
				results = append(results, map[string]interface{}{
					"orderNo":     orderNo,
					"terminalNo":  terminalNo,
					"orderStatus": orderStatus,
					"totalAmount": totalAmount,
				})
			}
			json.NewEncoder(w).Encode(results)
			return
		}

		// POST /api/orders (注文登録)
		if r.Method == http.MethodPost {
			var req OrderCreateReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "JSONのパースに失敗しました", http.StatusBadRequest)
				return
			}

			// 入力チェック (簡易版)
			if req.TerminalNo == "" || req.MessageType != "ORDER_CONFIRM" || len(req.Items) == 0 {
				http.Error(w, "入力チェックエラーです", http.StatusBadRequest)
				return
			}

			orderNo, err := generateOrderNo(database)
			if err != nil {
				http.Error(w, "注文番号の生成に失敗しました", http.StatusInternalServerError)
				return
			}

			initialStatus := "オーダー受信"
			for i, item := range req.Items {
				subtotal := item.UnitPrice * item.Quantity
				query := `INSERT INTO order_items (order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
				_, err = database.Exec(query, orderNo, req.TerminalNo, initialStatus, i+1, item.MenuName, item.UnitPrice, item.Quantity, subtotal)
				if err != nil {
					http.Error(w, "DB保存失敗", http.StatusInternalServerError)
					return
				}
			}

			res := map[string]interface{}{
				"result":      "OK",
				"orderNo":     orderNo,
				"orderStatus": initialStatus,
				"totalAmount": req.TotalAmount,
				"message":     "注文を受け付けました",
			}
			writeLog("DB登録内容", res)
			json.NewEncoder(w).Encode(res)
			return
		}

		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// 2. 注文詳細 & 状態更新 ハンドラー (パスパラメータの手動解析)
func OrderDetailHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// URLの末尾からorderNoやstatusの判定を行う (/api/orders/{orderNo}/status も考慮)
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathParts) < 3 {
			http.Error(w, "不正なURLです", http.StatusBadRequest)
			return
		}
		orderNo := pathParts[2] // {orderNo} 部分

		// PUT /api/orders/{orderNo}/status (注文状態更新)
		if r.Method == http.MethodPut && len(pathParts) == 4 && pathParts[3] == "status" {
			var req struct {
				OrderStatus string `json:"orderStatus"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			// 実証済みのバリデーションロジック
			if req.OrderStatus != "オーダー受信" &&
				req.OrderStatus != "クッキング終了" &&
				req.OrderStatus != "受け渡し終了" {
				http.Error(w, "orderStatus が不正です", http.StatusBadRequest)
				return
			}

			_, err := database.Exec("UPDATE order_items SET order_status = ? WHERE order_no = ?", req.OrderStatus, orderNo)
			if err != nil {
				http.Error(w, "DB更新失敗", http.StatusInternalServerError)
				return
			}

			res := map[string]interface{}{
				"result":      "OK",
				"orderNo":     orderNo,
				"orderStatus": req.OrderStatus,
				"message":     "注文状態を更新しました",
			}
			writeLog("DB更新内容", res)
			json.NewEncoder(w).Encode(res)
			return
		}

		// GET /api/orders/{orderNo} (詳細取得)
		if r.Method == http.MethodGet && len(pathParts) == 3 {
			rows, err := database.Query(`SELECT order_no, order_status, item_no, menu_name, unit_price, quantity, subtotal, created_at FROM order_items WHERE order_no = ?`, orderNo)
			if err != nil {
				http.Error(w, "詳細取得失敗", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			type ItemDetail struct {
				CreatedAt   string `json:"createdAt"`
				ItemNo      int    `json:"itemNo"`
				MenuName    string `json:"menuName"`
				OrderStatus string `json:"orderStatus"`
				Quantity    int    `json:"quantity"`
				Subtotal    int    `json:"subtotal"`
				UnitPrice   int    `json:"unitPrice"`
			}

			var items []ItemDetail
			for rows.Next() {
				var oNo, oStatus, menu, cAt string
				var itemNo, price, qty, sub int
				rows.Scan(&oNo, &oStatus, &itemNo, &menu, &price, &qty, &sub, &cAt)

				items = append(items, ItemDetail{
					CreatedAt:   cAt,
					ItemNo:      itemNo,
					MenuName:    menu,
					OrderStatus: oStatus,
					Quantity:    qty,
					Subtotal:    sub,
					UnitPrice:   price,
				})
			}

			if len(items) == 0 {
				http.Error(w, "注文が見つかりません", http.StatusNotFound)
				return
			}

			res := map[string]interface{}{
				"orderNo": orderNo,
				"items":   items,
			}
			json.NewEncoder(w).Encode(res)
			return
		}

		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// 3. フロント掲示板ハンドラー (GET & PUT /api/board)
func BoardHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 掲示板データ送出用のインナールーティン
		sendBoardData := func() {
			cookingOrders := []string{}
			readyOrders := []string{}

			rows, err := database.Query(`SELECT order_no, order_status FROM order_items WHERE order_status IN ('オーダー受信', 'クッキング終了') GROUP BY order_no`)
			if err != nil {
				http.Error(w, "DBエラー", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			for rows.Next() {
				var orderNo, status string
				rows.Scan(&orderNo, &status)
				if status == "オーダー受信" {
					cookingOrders = append(cookingOrders, orderNo)
				} else if status == "クッキング終了" {
					readyOrders = append(readyOrders, orderNo)
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"result":        "OK",
				"cookingOrders": cookingOrders,
				"readyOrders":   readyOrders,
			})
		}

		// GET /api/board
		if r.Method == http.MethodGet {
			sendBoardData()
			return
		}

		// PUT /api/board (商品受け渡し時)
		if r.Method == http.MethodPut {
			var req struct {
				OrderNo string `json:"orderNo"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "JSON不正", http.StatusBadRequest)
				return
			}

			_, err := database.Exec(`UPDATE order_items SET order_status = '受け渡し終了' WHERE order_no = ? AND order_status = 'クッキング終了'`, req.OrderNo)
			if err != nil {
				http.Error(w, "更新失敗", http.StatusInternalServerError)
				return
			}

			writeLog("掲示板更新", req.OrderNo)
			sendBoardData()
			return
		}
	}
}

// 4. 厨房機能ハンドラー (GET & PUT /api/kitchen)
func KitchenHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sendKitchenData := func() {
			rows, err := database.Query(`SELECT order_no, menu_name, quantity FROM order_items WHERE order_status = 'オーダー受信' ORDER BY created_at ASC`)
			if err != nil {
				http.Error(w, "DBエラー", http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			orderMap := make(map[string][]KitchenItem)
			orderOrder := []string{}

			for rows.Next() {
				var orderNo string
				var item KitchenItem
				rows.Scan(&orderNo, &item.MenuName, &item.Quantity)

				if _, exists := orderMap[orderNo]; !exists {
					orderOrder = append(orderOrder, orderNo)
				}
				orderMap[orderNo] = append(orderMap[orderNo], item)
			}

			orders := []KitchenOrder{}
			for _, orderNo := range orderOrder {
				orders = append(orders, KitchenOrder{OrderNo: orderNo, Items: orderMap[orderNo]})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"result": "OK", "orders": orders})
		}

		// GET /api/kitchen
		if r.Method == http.MethodGet {
			sendKitchenData()
			return
		}

		// PUT /api/kitchen (調理完了時)
		if r.Method == http.MethodPut {
			var req struct {
				OrderNo string `json:"orderNo"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			_, err := database.Exec(`UPDATE order_items SET order_status = 'クッキング終了' WHERE order_no = ? AND order_status = 'オーダー受信'`, req.OrderNo)
			if err != nil {
				http.Error(w, "更新失敗", http.StatusInternalServerError)
				return
			}

			writeLog("厨房更新", req.OrderNo)
			sendKitchenData()
			return
		}
	}
}