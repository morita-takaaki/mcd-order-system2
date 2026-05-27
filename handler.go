package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// --- 構造体の定義 ---

type OrderItemReq struct {
	MenuName  string `json:"menuName"`
	UnitPrice int    `json:"unitPrice"`
	Quantity  int    `json:"quantity"`
}

type OrderRequest struct {
	TerminalNo  string         `json:"terminalNo"`
	MessageType string         `json:"messageType"`
	TotalAmount int            `json:"totalAmount"`
	Items       []OrderItemReq `json:"items"`
}

type OrderResponse struct {
	Result      string `json:"result"`
	OrderNo     string `json:"orderNo"`
	OrderStatus string `json:"orderStatus"`
	TotalAmount int    `json:"totalAmount"`
	Message     string `json:"message"`
}

type OrderSummary struct {
	OrderNo     string `json:"orderNo"`
	TerminalNo  string `json:"terminalNo"`
	OrderStatus string `json:"orderStatus"`
	TotalAmount int    `json:"totalAmount"`
	CreatedAt   string `json:"createdAt"`
}

type OrderDetailResponse struct {
	OrderNo     string      `json:"orderNo"`
	TerminalNo  string      `json:"terminalNo"`
	OrderStatus string      `json:"orderStatus"`
	CreatedAt   string      `json:"createdAt"`
	Items       []DBItemOut `json:"items"`
}

type DBItemOut struct {
	ItemNo    int    `json:"itemNo"`
	MenuName  string `json:"menuName"`
	UnitPrice int    `json:"unitPrice"`
	Quantity  int    `json:"quantity"`
	Subtotal  int    `json:"subtotal"`
}

type StatusUpdateRequest struct {
	OrderStatus string `json:"orderStatus"`
}

// --- ハンドラー関数 ---

// /api/orders に対するルーティング分岐 (POST / GET)
func handleOrders(w http.ResponseWriter, r *http.Request) { // http.Context から修正
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		createOrder(w, r)
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		if status != "" {
			getOrdersByStatus(w, status)
		} else {
			getAllOrders(w, r) // 引数に r を追加して修正
		}
	default:
		http.Error(w, `{"error":"Method Not Allowed"}`, http.StatusMethodNotAllowed)
	}
}

// /api/orders/{orderNo} 形式のURLのルーティング分岐 (GET / PUT)
func handleOrderWithParam(w http.ResponseWriter, r *http.Request) { // http.Context から修正
	w.Header().Set("Content-Type", "application/json")

	// URLからパラメータを抽出 (/api/orders/0424-001 -> 0424-001)
	pathTrimmed := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	parts := strings.Split(pathTrimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, `{"error":"Bad Request"}`, http.StatusBadRequest)
		return
	}
	orderNo := parts[0]

	// パスが "/api/orders/{orderNo}/status" の形かチェック
	isStatusPath := len(parts) == 2 && parts[1] == "status"

	switch r.Method {
	case http.MethodGet:
		getOrderDetail(w, orderNo)
	case http.MethodPut:
		if isStatusPath {
			updateOrderStatus(w, r, orderNo)
		} else {
			http.Error(w, `{"error":"Not Found"}`, http.StatusNotFound)
		}
	default:
		http.Error(w, `{"error":"Method Not Allowed"}`, http.StatusMethodNotAllowed)
	}
}

// 4.1 注文登録処理 (POST /api/orders)
func createOrder(w http.ResponseWriter, r *http.Request) { // http.Context から修正
	// ログ用に入電文（生JSON）を読み込む
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // 読み直せるように戻す

	var req OrderRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// --- 入力チェック (バリデーション) ---
	if req.TerminalNo == "" {
		http.Error(w, `{"error":"terminalNoは必須です"}`, http.StatusBadRequest)
		return
	}
	if req.MessageType != "ORDER_CONFIRM" {
		http.Error(w, `{"error":"messageTypeはORDER_CONFIRM固定です"}`, http.StatusBadRequest)
		return
	}
	if req.TotalAmount < 1 {
		http.Error(w, `{"error":"totalAmountは1以上である必要があります"}`, http.StatusBadRequest)
		return
	}
	if len(req.Items) < 1 || len(req.Items) > 5 {
		http.Error(w, `{"error":"itemsは1〜5件にしてください"}`, http.StatusBadRequest)
		return
	}

	menuMap := make(map[string]bool)
	calculatedTotal := 0
	calculatedItems := make([]DBItemOut, len(req.Items))

	for i, item := range req.Items {
		if item.MenuName == "" {
			http.Error(w, `{"error":"menuNameは必須です"}`, http.StatusBadRequest)
			return
		}
		if menuMap[item.MenuName] {
			http.Error(w, `{"error":"menuNameが重複しています"}`, http.StatusBadRequest)
			return
		}
		menuMap[item.MenuName] = true

		if item.UnitPrice < 1 {
			http.Error(w, `{"error":"unitPriceは1以上です"}`, http.StatusBadRequest)
			return
		}
		if item.Quantity < 1 || item.Quantity > 5 {
			http.Error(w, `{"error":"quantityは1〜5の範囲です"}`, http.StatusBadRequest)
			return
		}

		// 小計 (subtotal) の自動計算
		subtotal := item.UnitPrice * item.Quantity
		calculatedTotal += subtotal

		calculatedItems[i] = DBItemOut{
			ItemNo:    i + 1,
			MenuName:  item.MenuName,
			UnitPrice: item.UnitPrice,
			Quantity:  item.Quantity,
			Subtotal:  subtotal,
		}
	}

	// 合計金額の一致チェック
	if calculatedTotal != req.TotalAmount {
		http.Error(w, `{"error":"totalAmountが小計の合計と一致しません"}`, http.StatusBadRequest)
		return
	}

	// 注文番号の採番
	orderNo, err := generateOrderNo()
	if err != nil {
		http.Error(w, `{"error":"注文番号の採番に失敗しました"}`, http.StatusInternalServerError)
		return
	}
	orderStatus := "オーダー受信"

	// --- DB保存処理とログ用データ組み立て ---
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"トランザクションの開始に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	var dbLogItems []string
	for _, item := range calculatedItems {
		query := `INSERT INTO order_items (order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := tx.Exec(query, orderNo, req.TerminalNo, orderStatus, item.ItemNo, item.MenuName, item.UnitPrice, item.Quantity, item.Subtotal)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error":"DB登録に失敗しました"}`, http.StatusInternalServerError)
			return
		}
		dbLogItems = append(dbLogItems, fmt.Sprintf("(No:%d, %s, %d円×%d=%d円)", item.ItemNo, item.MenuName, item.UnitPrice, item.Quantity, item.Subtotal))
	}
	tx.Commit()

	// レスポンスの組み立て
	res := OrderResponse{
		Result:      "OK",
		OrderNo:     orderNo,
		OrderStatus: orderStatus,
		TotalAmount: req.TotalAmount,
		Message:     "注文を受け付けました",
	}
	resBytes, _ := json.Marshal(res)

	// --- 要件に基づく1注文単位のログ出力 ---
	log.Println("==================== 新規注文処理 ====================")
	log.Printf("[入電文] %s\n", string(bodyBytes))
	log.Printf("[DB登録内容] order_no: %s, terminal_no: %s, status: %s, 明細: %s\n", orderNo, req.TerminalNo, orderStatus, strings.Join(dbLogItems, ", "))
	log.Printf("[出電文] %s\n", string(resBytes))
	log.Println("=====================================================")

	w.Write(resBytes)
}

// 4.2 参照処理: 注文一覧取得 (GET /api/orders)
func getAllOrders(w http.ResponseWriter, r *http.Request) { // http.Context から修正、第二引数追加
	// order_no単位で集約（SUMで合計金額を計算、MAXで基本情報を取得）
	query := `
		SELECT order_no, max(terminal_no), max(order_status), sum(subtotal), max(created_at)
		FROM order_items
		GROUP BY order_no
		ORDER BY max(created_at) DESC`

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, `{"error":"データ取得に失敗しました"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	orders := []OrderSummary{}
	for rows.Next() {
		var o OrderSummary
		if err := rows.Scan(&o.OrderNo, &o.TerminalNo, &o.OrderStatus, &o.TotalAmount, &o.CreatedAt); err == nil {
			orders = append(orders, o)
		}
	}

	json.NewEncoder(w).Encode(orders)
}

// 4.2 参照処理: 状態別一覧取得 (GET /api/orders?status=xxx)
func getOrdersByStatus(w http.ResponseWriter, status string) {
	query := `
		SELECT order_no, max(terminal_no), max(order_status), sum(subtotal), max(created_at)
		FROM order_items
		WHERE order_status = ?
		GROUP BY order_no
		ORDER BY max(created_at) DESC`

	rows, err := db.Query(query, status)
	if err != nil {
		http.Error(w, `{"error":"データ取得に失敗しました"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	orders := []OrderSummary{}
	for rows.Next() {
		var o OrderSummary
		if err := rows.Scan(&o.OrderNo, &o.TerminalNo, &o.OrderStatus, &o.TotalAmount, &o.CreatedAt); err == nil {
			orders = append(orders, o)
		}
	}

	json.NewEncoder(w).Encode(orders)
}

// 4.2 参照処理: 注文詳細取得 (GET /api/orders/{orderNo})
func getOrderDetail(w http.ResponseWriter, orderNo string) {
	query := `SELECT terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal, created_at FROM order_items WHERE order_no = ? ORDER BY item_no ASC`
	rows, err := db.Query(query, orderNo)
	if err != nil {
		http.Error(w, `{"error":"データ取得に失敗しました"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var res OrderDetailResponse
	res.OrderNo = orderNo
	res.Items = []DBItemOut{}

	isFirst := true
	for rows.Next() {
		var item DBItemOut
		var terminalNo, orderStatus, createdAt string
		err := rows.Scan(&terminalNo, &orderStatus, &item.ItemNo, &item.MenuName, &item.UnitPrice, &item.Quantity, &item.Subtotal, &createdAt)
		if err != nil {
			continue
		}
		if isFirst {
			res.TerminalNo = terminalNo
			res.OrderStatus = orderStatus
			res.CreatedAt = createdAt
			isFirst = false
		}
		res.Items = append(res.Items, item)
	}

	// 該当する注文番号が一件もない場合
	if isFirst {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"指定された注文番号は見つかりません"}`))
		return
	}

	json.NewEncoder(w).Encode(res)
}

// 4.3 更新処理: 注文状態更新 (PUT /api/orders/{orderNo}/status)
func updateOrderStatus(w http.ResponseWriter, r *http.Request, orderNo string) { // http.Context から修正
	var req StatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.OrderStatus == "" {
		http.Error(w, `{"error":"orderStatusは必須です"}`, http.StatusBadRequest)
		return
	}

	// order_noに紐づくすべての行を更新
	query := `UPDATE order_items SET order_status = ? WHERE order_no = ?`
	result, err := db.Exec(query, req.OrderStatus, orderNo)
	if err != nil {
		http.Error(w, `{"error":"状態の更新に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"指定された注文番号は見つかりません"}`))
		return
	}

	// レスポンス送信
	w.Write([]byte(fmt.Sprintf(`{"result":"OK","message":"注文番号 %s の状態を \'%s\' に更新しました"}`, orderNo, req.OrderStatus)))

	// --- 更新ログの出力 ---
	log.Println("==================== 注文状態更新 ====================")
	log.Printf("[DB更新内容] order_no: %s の状態を '%s' に変更しました(影響行数:%d)\n", orderNo, req.OrderStatus, rowsAffected)
	log.Println("=====================================================")
}