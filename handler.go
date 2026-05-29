package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// --- 構造体の定義 ---

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

type OrderCreateRes struct {
	Result      string `json:"result"`
	OrderNo     string `json:"orderNo"`
	OrderStatus string `json:"orderStatus"`
	TotalAmount int    `json:"totalAmount"`
	Message     string `json:"message"`
}

type OrderSummary struct {
	OrderNo     string    `json:"orderNo"`
	TerminalNo  string    `json:"terminalNo"`
	OrderStatus string    `json:"orderStatus"`
	TotalAmount int       `json:"totalAmount"`
	CreatedAt   time.Time `json:"createdAt"`
}

type KitchenItem struct {
	MenuName string `json:"menuName"`
	Quantity int    `json:"quantity"`
}

type KitchenOrder struct {
	OrderNo string        `json:"orderNo"`
	Items   []KitchenItem `json:"items"`
}

type KitchenRes struct {
	Result string         `json:"result"`
	Orders []KitchenOrder `json:"orders"`
}

type BoardRes struct {
	Result        string   `json:"result"`
	CookingOrders []string `json:"cookingOrders"`
	ReadyOrders   []string `json:"readyOrders"`
}

type UpdateReq struct {
	OrderNo string `json:"orderNo"`
}

type ErrorRes struct {
	Result  string `json:"result"`
	Message string `json:"message"`
}

// --- ユーティリティ関数 ---

// respondWithError はエラーレスポンスをJSONで返却しログに出力します
func respondWithError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	res := ErrorRes{Result: "NG", Message: message}
	json.NewEncoder(w).Encode(res)
	resBytes, _ := json.Marshal(res)
	log.Printf("[RES] Status: %d, Body: %s", code, string(resBytes))
}

// respondWithJSON は成功レスポンスをJSONで返却しログに出力します
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
	resBytes, _ := json.Marshal(payload)
	log.Printf("[RES] Status: %d, Body: %s", code, string(resBytes))
}

// generateOrderNo は当日（MMDD）の最新連番をDBから取得して採番します
func generateOrderNo() (string, error) {
	today := time.Now().Format("0102") // MMDD形式
	var maxOrderNo sql.NullString

	// 当日の最大の注文番号を取得
	err := db.QueryRow("SELECT MAX(order_no) FROM order_items WHERE order_no LIKE ?", today+"-%").Scan(&maxOrderNo)
	if err != nil {
		return "", err
	}

	nextNum := 1
	if maxOrderNo.Valid {
		// "0523-001" の後ろ3桁をパース
		var lastNum int
		_, err := fmt.Sscanf(maxOrderNo.String, today+"-%d", &lastNum)
		if err == nil {
			nextNum = lastNum + 1
		}
	}

	return fmt.Sprintf("%s-%03d", today, nextNum), nil
}

// --- ハンドラー関数の実装 ---

// 5.1 注文処理 (POST /api/orders)
func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	log.Printf("[BODY] %s", string(bodyBytes))

	var req OrderCreateReq
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSONのパースに失敗しました")
		return
	}

	// 8.2 入力チェック
	if req.TerminalNo == "" {
		respondWithError(w, http.StatusBadRequest, "terminalNoは必須です")
		return
	}
	if req.MessageType != "ORDER_CONFIRM" {
		respondWithError(w, http.StatusBadRequest, "messageTypeはORDER_CONFIRMである必要があります")
		return
	}
	if req.TotalAmount < 1 {
		respondWithError(w, http.StatusBadRequest, "totalAmountは1以上である必要があります")
		return
	}
	if len(req.Items) < 1 || len(req.Items) > 5 {
		respondWithError(w, http.StatusBadRequest, "itemsは1〜5件である必要があります")
		return
	}

	calcTotal := 0
	menuMap := make(map[string]bool)

	for _, item := range req.Items {
		if item.MenuName == "" {
			respondWithError(w, http.StatusBadRequest, "menuNameは必須です")
			return
		}
		if item.UnitPrice < 1 {
			respondWithError(w, http.StatusBadRequest, "unitPriceは1以上である必要があります")
			return
		}
		if item.Quantity < 1 || item.Quantity > 5 {
			respondWithError(w, http.StatusBadRequest, "quantityは1〜5である必要があります")
			return
		}
		if menuMap[item.MenuName] {
			respondWithError(w, http.StatusBadRequest, "menuNameが重複しています")
			return
		}
		menuMap[item.MenuName] = true
		calcTotal += item.UnitPrice * item.Quantity
	}

	if calcTotal != req.TotalAmount {
		respondWithError(w, http.StatusBadRequest, "totalAmountが計算結果と一致しません")
		return
	}

	// 7.1 採番
	orderNo, err := generateOrderNo()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "注文番号の生成に失敗しました")
		return
	}

	// DBへ保存
	status := "オーダ受信済み"
	for i, item := range req.Items {
		subtotal := item.UnitPrice * item.Quantity
		query := `INSERT INTO order_items (order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := db.Exec(query, orderNo, req.TerminalNo, status, i+1, item.MenuName, item.UnitPrice, item.Quantity, subtotal)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "DBへの保存に失敗しました")
			return
		}
		log.Printf("[DB INSERT] OrderNo: %s, Item: %s", orderNo, item.MenuName)
	}

	res := OrderCreateRes{
		Result:      "OK",
		OrderNo:     orderNo,
		OrderStatus: status,
		TotalAmount: req.TotalAmount,
		Message:     "注文を受け付けました",
	}
	respondWithJSON(w, http.StatusOK, res)
}

// 5.2 参照処理: 注文一覧取得 (GET /api/orders)
func handleGetOrders(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	var rows *sql.Rows
	var err error

	if statusFilter != "" {
		query := `SELECT order_no, terminal_no, order_status, SUM(subtotal) FROM order_items WHERE order_status = ? GROUP BY order_no ORDER BY created_at DESC`
		rows, err = db.Query(query, statusFilter)
	} else {
		query := `SELECT order_no, terminal_no, order_status, SUM(subtotal) FROM order_items GROUP BY order_no ORDER BY created_at DESC`
		rows, err = db.Query(query)
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "データ取得に失敗しました")
		return
	}
	defer rows.Close()

	summaries := []OrderSummary{}
	for rows.Next() {
		var s OrderSummary
		if err := rows.Scan(&s.OrderNo, &s.TerminalNo, &s.OrderStatus, &s.TotalAmount); err != nil {
			respondWithError(w, http.StatusInternalServerError, "データの読み込みに失敗しました")
			return
		}
		summaries = []OrderSummary{s} // 単一スライスの簡易割り当て（追加時は append を使用）
		_ = append(summaries, s)     // 教材用としてのサマライズ
	}

	// 実際の全件アペンド
	var actualSummaries []OrderSummary
	// rowsを再利用できないため、本来は一括でスキャンします（以下は確定版集約）
	queryAll := `SELECT order_no, terminal_no, order_status, SUM(subtotal) FROM order_items `
	var args []interface{}
	if statusFilter != "" {
		queryAll += "WHERE order_status = ? "
		args = append(args, statusFilter)
	}
	queryAll += "GROUP BY order_no ORDER BY created_at ASC"
	
	r2, _ := db.Query(queryAll, args...)
	defer r2.Close()
	for r2.Next() {
		var s OrderSummary
		r2.Scan(&s.OrderNo, &s.TerminalNo, &s.OrderStatus, &s.TotalAmount)
		actualSummaries = append(actualSummaries, s)
	}

	respondWithJSON(w, http.StatusOK, actualSummaries)
}

// 5.2 参照処理: 詳細取得 (GET /api/orders/{orderNo})
func handleGetOrderDetail(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")

	query := `SELECT menu_name, quantity, unit_price, subtotal FROM order_items WHERE order_no = ?`
	rows, err := db.Query(query, orderNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "詳細データの取得に失敗しました")
		return
	}
	defer rows.Close()

	type DetailItem struct {
		MenuName  string `json:"menuName"`
		Quantity  int    `json:"quantity"`
		UnitPrice int    `json:"unitPrice"`
		Subtotal  int    `json:"subtotal"`
	}
	var items []DetailItem

	for rows.Next() {
		var item DetailItem
		if err := rows.Scan(&item.MenuName, &item.Quantity, &item.UnitPrice, &item.Subtotal); err != nil {
			respondWithError(w, http.StatusInternalServerError, "データのパースに失敗しました")
			return
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		respondWithError(w, http.StatusNotFound, "指定された注文が見つかりません")
		return
	}

	respondWithJSON(w, http.StatusOK, items)
}

// 5.3 更新処理: 注文状態変更 (PUT /api/orders/{orderNo}/status)
func handleUpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSONが不正です")
		return
	}

	if req.Status != "オーダ受信済み" && req.Status != "調理済み" && req.Status != "受け渡し済み" {
		respondWithError(w, http.StatusBadRequest, "無効なステータス値です")
		return
	}

	query := `UPDATE order_items SET order_status = ? WHERE order_no = ?`
	result, err := db.Exec(query, req.Status, orderNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "ステータスの更新に失敗しました")
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		respondWithError(w, http.StatusNotFound, "該当する注文が存在しません")
		return
	}

	log.Printf("[DB UPDATE] OrderNo: %s, NewStatus: %s", orderNo, req.Status)
	respondWithJSON(w, http.StatusOK, map[string]string{"result": "OK", "message": "ステータスを更新しました"})
}

// 5.4 (1) フロント掲示板参照 (GET /api/board)
func handleGetBoard(w http.ResponseWriter, r *http.Request) {
	cookingOrders := []string{}
	readyOrders := []string{}

	query := `SELECT order_no, order_status FROM order_items WHERE order_status IN ('オーダ受信済み', '調理済み') GROUP BY order_no`
	rows, err := db.Query(query)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "掲示板データの取得に失敗しました")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var orderNo, status string
		rows.Scan(&orderNo, &status)
		if status == "オーダ受信済み" {
			cookingOrders = append(cookingOrders, orderNo)
		} else if status == "調理済み" {
			readyOrders = append(readyOrders, orderNo)
		}
	}

	respondWithJSON(w, http.StatusOK, BoardRes{
		Result:        "OK",
		CookingOrders: cookingOrders,
		ReadyOrders:   readyOrders,
	})
}

// 5.4 (2) フロント掲示板更新 (PUT /api/board)
func handleUpdateBoard(w http.ResponseWriter, r *http.Request) {
	var req UpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSONが不正です")
		return
	}

	query := `UPDATE order_items SET order_status = '受け渡し済み' WHERE order_no = ? AND order_status = '調理済み'`
	_, err := db.Exec(query, req.OrderNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "DBの更新に失敗しました")
		return
	}
	log.Printf("[DB UPDATE BOARD] OrderNo: %s -> 受け渡し済み", req.OrderNo)

	// 最新の掲示板情報を返却
	handleGetBoard(w, r)
}

// 5.5 (1) 厨房参照 (GET /api/kitchen)
func handleGetKitchen(w http.ResponseWriter, r *http.Request) {
	query := `SELECT order_no, menu_name, quantity FROM order_items WHERE order_status = 'オーダ受信済み' ORDER BY created_at ASC`
	rows, err := db.Query(query)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "厨房データの取得に失敗しました")
		return
	}
	defer rows.Close()

	orderMap := make(map[string][]KitchenItem)
	var orderOrder []string // 順序を保持するためのスライス

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
		orders = append(orders, KitchenOrder{
			OrderNo: orderNo,
			Items:   orderMap[orderNo],
		})
	}

	respondWithJSON(w, http.StatusOK, KitchenRes{Result: "OK", Orders: orders})
}

// 5.5 (2) 厨房更新 (PUT /api/kitchen)
func handleUpdateKitchen(w http.ResponseWriter, r *http.Request) {
	var req UpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSONが不正です")
		return
	}

	query := `UPDATE order_items SET order_status = '調理済み' WHERE order_no = ? AND order_status = 'オーダ受信済み'`
	_, err := db.Exec(query, req.OrderNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "DBの更新に失敗しました")
		return
	}
	log.Printf("[DB UPDATE KITCHEN] OrderNo: %s -> 調理済み", req.OrderNo)

	// 最新の厨房情報を返却
	handleGetKitchen(w, r)
}