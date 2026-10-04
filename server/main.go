package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var exchange *Exchange

// --- WebSocket ---

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WsClient struct {
	conn    *websocket.Conn
	userID  string
	isAdmin bool
	mu      sync.Mutex
}

var (
	wsClients   = make(map[*WsClient]bool)
	wsClientsMu sync.Mutex
)

func broadcastAll() {
	wsClientsMu.Lock()
	clients := make([]*WsClient, 0, len(wsClients))
	for c := range wsClients {
		clients = append(clients, c)
	}
	wsClientsMu.Unlock()

	if len(clients) == 0 { return }

	shared := exchange.ComputeSharedState()

	for _, c := range clients {
		go func(c *WsClient) {
			if c.userID == "" { return }
			state := exchange.BuildUserState(c.userID, c.isAdmin, &shared)
			if state == nil { return }
			msg := map[string]interface{}{"type": "state", "data": state}
			data, err := json.Marshal(msg)
			if err != nil { return }
			c.mu.Lock()
			defer c.mu.Unlock()
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			c.conn.WriteMessage(websocket.TextMessage, data)
		}(c)
	}
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }

	client := &WsClient{conn: conn}
	wsClientsMu.Lock()
	wsClients[client] = true
	wsClientsMu.Unlock()

	defer func() {
		wsClientsMu.Lock()
		delete(wsClients, client)
		wsClientsMu.Unlock()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil { break }

		var payload map[string]string
		if json.Unmarshal(msg, &payload) != nil { continue }

		if payload["type"] == "auth" {
			token := payload["token"]
			u, err := exchange.AuthenticateR(token)
			if err != nil {
				errMsg, _ := json.Marshal(map[string]string{"type": "error", "error": "invalid session"})
				client.mu.Lock()
				conn.WriteMessage(websocket.TextMessage, errMsg)
				client.mu.Unlock()
				continue
			}
			client.userID = u.ID
			client.isAdmin = u.IsAdmin

			state := exchange.GetFullState(u.ID, u.IsAdmin)
			stateMsg, _ := json.Marshal(map[string]interface{}{"type": "state", "data": state})
			client.mu.Lock()
			conn.WriteMessage(websocket.TextMessage, stateMsg)
			client.mu.Unlock()
		}
	}
}

// --- HTTP Helpers ---

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func jsonResp(w http.ResponseWriter, data interface{}, status int) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func errResp(w http.ResponseWriter, msg string, status int) {
	jsonResp(w, map[string]string{"error": msg}, status)
}

func getToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

func readBody(r *http.Request) map[string]interface{} {
	body, err := io.ReadAll(r.Body)
	if err != nil { return nil }
	var m map[string]interface{}
	json.Unmarshal(body, &m)
	return m
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok { return s }
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok { return f }
	}
	return 0
}

func getInt(m map[string]interface{}, key string) int {
	return int(getFloat(m, key))
}

// --- Routes ---

func main() {
	exchange = NewExchange("exchange_data.json")
	exchange.broadcast = broadcastAll

	if err := exchange.Load(); err != nil {
		log.Printf("Warning: could not load state: %v", err)
	}
	exchange.rec = NewRecorder("data")
	exchange.mu.Lock()
	exchange.ensureProject()
	exchange.mu.Unlock()

	// Exchange clock: runs just after every wall-clock second boundary for the second that ended.
	go func() {
		for {
			now := time.Now()
			next := now.Truncate(time.Second).Add(time.Second)
			time.Sleep(time.Until(next) + 2*time.Millisecond)
			exchange.OnSecond(next.Unix() - 1)
		}
	}()

	// House bots
	go func() {
		for range time.Tick(time.Duration(simTickSec * float64(time.Second))) {
			exchange.SimTick()
		}
	}()

	// Market data files
	go func() {
		for range time.Tick(60 * time.Second) {
			exchange.rec.Flush()
		}
	}()

	// Broadcast updated state every second (PnL changes with price even without trades)
	go func() {
		for range time.Tick(1 * time.Second) {
			broadcastAll()
		}
	}()

	// Snapshot prices/PnL for charts every 5s (doesn't need 1s resolution)
	go func() {
		for range time.Tick(5 * time.Second) {
			exchange.SnapshotPnl()
			exchange.SnapshotPrices()
		}
	}()

	// Periodic save (only writes if state changed)
	go func() {
		for range time.Tick(5 * time.Second) {
			exchange.SaveIfDirty()
		}
	}()

	mux := http.NewServeMux()

	// WebSocket
	mux.HandleFunc("/ws", handleWs)

	// Auth
	mux.HandleFunc("/api/auth", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }

		body := readBody(r)
		username := getString(body, "username")
		password := getString(body, "password")
		if username == "" || password == "" {
			errResp(w, "username and password required", 400); return
		}

		u, err := exchange.Login(username, password)
		if err != nil {
			u, err = exchange.Register(username, password)
			if err != nil {
				errResp(w, err.Error(), 401); return
			}
		}

		jsonResp(w, map[string]interface{}{
			"token": u.SessionToken, "userId": u.ID,
			"username": u.Username, "isAdmin": u.IsAdmin,
		}, 200)
	})

	// State
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		token := getToken(r)
		if token == "" { errResp(w, "missing bearer token", 401); return }
		u, err := exchange.AuthenticateR(token)
		if err != nil { errResp(w, err.Error(), 401); return }
		state := exchange.GetBotState(u.ID)
		jsonResp(w, state, 200)
	})

	// Place order
	mux.HandleFunc("/api/order", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }

		token := getToken(r)
		if token == "" { errResp(w, "missing bearer token", 401); return }

		if r.Method == "DELETE" {
			body := readBody(r)
			orderID := getString(body, "orderId")
			if orderID == "" { errResp(w, "orderId required", 400); return }
			if err := exchange.CancelOrder(token, orderID); err != nil {
				errResp(w, err.Error(), 400); return
			}
			jsonResp(w, map[string]bool{"success": true}, 200)
			return
		}

		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }

		body := readBody(r)
		symbol := getString(body, "symbol")
		side := getString(body, "side")
		price := getFloat(body, "price")
		qty := getInt(body, "qty")
		orderType := getString(body, "type")
		if orderType == "" { orderType = "limit" }
		if orderType == "postOnly" || getString(body, "orderType") == "postOnly" {
			orderType = "postOnly"
		} else if orderType == "ioc" || getString(body, "orderType") == "ioc" {
			orderType = "ioc"
		} else {
			orderType = "limit"
		}

		productID := resolveProduct(symbol)
		if productID == "" { errResp(w, fmt.Sprintf("unknown symbol: %s", symbol), 404); return }

		result, err := exchange.PlaceOrder(token, productID, side, price, qty, orderType)
		if err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, result, 200)
	})

	// Cancel all
	mux.HandleFunc("/api/cancel-all", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }

		token := getToken(r)
		if token == "" { errResp(w, "missing bearer token", 401); return }

		body := readBody(r)
		var productID *string
		var side *string
		if sym := getString(body, "symbol"); sym != "" {
			pid := resolveProduct(sym)
			if pid == "" { errResp(w, fmt.Sprintf("unknown symbol: %s", sym), 404); return }
			productID = &pid
		}
		if s := getString(body, "side"); s != "" {
			side = &s
		}

		count, err := exchange.CancelAll(token, productID, side)
		if err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, map[string]int{"cancelled": count}, 200)
	})

	// Batch
	mux.HandleFunc("/api/batch", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }

		token := getToken(r)
		if token == "" { errResp(w, "missing bearer token", 401); return }

		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Actions []BatchAction `json:"actions"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			errResp(w, "invalid JSON", 400); return
		}

		for i := range payload.Actions {
			a := &payload.Actions[i]
			if a.Symbol != "" && a.ProductID == "" {
				pid := resolveProduct(a.Symbol)
				if pid != "" {
					a.ProductID = pid
				}
			}
			if a.Type == "order" && a.OrderType == "" {
				a.OrderType = "limit"
			}
		}

		results := exchange.ExecuteBatch(token, payload.Actions)
		jsonResp(w, results, 200)
	})

	// ETF swap
	mux.HandleFunc("/api/etf-swap", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }

		token := getToken(r)
		if token == "" { errResp(w, "missing bearer token", 401); return }

		body := readBody(r)
		symbol := getString(body, "symbol")
		qty := getInt(body, "qty")
		direction := getString(body, "direction")
		compIdx := getInt(body, "compositionIndex")

		productID := resolveProduct(symbol)
		if productID == "" { errResp(w, fmt.Sprintf("unknown symbol: %s", symbol), 404); return }

		var prices []float64
		if p, ok := body["prices"]; ok && p != nil {
			if arr, ok := p.([]interface{}); ok {
				for _, v := range arr {
					if f, ok := v.(float64); ok { prices = append(prices, f) }
				}
			}
		}

		result, err := exchange.SwapEtf(token, productID, qty, direction, compIdx, prices)
		if err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, result, 200)
	})

	// Admin endpoints
	mux.HandleFunc("/api/admin/product", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }

		token := getToken(r)
		body := readBody(r)
		action := getString(body, "action")

		switch action {
		case "create":
			p, err := exchange.CreateProduct(token, getString(body, "symbol"), getString(body, "name"), getInt(body, "positionLimit"))
			if err != nil { errResp(w, err.Error(), 400); return }
			jsonResp(w, p, 200)
		case "createEtf":
			compsRaw, ok := body["compositions"].([]interface{})
			if !ok { errResp(w, "compositions required", 400); return }
			var compositions [][]EtfComponent
			for _, compRaw := range compsRaw {
				compArr, ok := compRaw.([]interface{})
				if !ok { continue }
				var comp []EtfComponent
				for _, c := range compArr {
					cm, ok := c.(map[string]interface{})
					if !ok { continue }
					comp = append(comp, EtfComponent{
						ProductID: getString(cm, "productId"),
						Weight:    getFloat(cm, "weight"),
					})
				}
				if len(comp) > 0 {
					compositions = append(compositions, comp)
				}
			}
			p, err := exchange.CreateEtf(token, getString(body, "symbol"), getString(body, "name"), getInt(body, "positionLimit"), compositions)
			if err != nil { errResp(w, err.Error(), 400); return }
			jsonResp(w, p, 200)
		case "delete":
			if err := exchange.DeleteProduct(token, getString(body, "productId")); err != nil {
				errResp(w, err.Error(), 400); return
			}
			jsonResp(w, map[string]bool{"success": true}, 200)
		case "updatePositionLimit":
			if err := exchange.UpdatePositionLimit(token, getString(body, "productId"), getInt(body, "positionLimit")); err != nil {
				errResp(w, err.Error(), 400); return
			}
			jsonResp(w, map[string]bool{"success": true}, 200)
		case "updateTickSize":
			if err := exchange.UpdateTickSize(token, getString(body, "productId"), getFloat(body, "tickSize")); err != nil {
				errResp(w, err.Error(), 400); return
			}
			jsonResp(w, map[string]bool{"success": true}, 200)
		default:
			errResp(w, "unknown action", 400)
		}
	})

	mux.HandleFunc("/api/admin/toggle", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		token := getToken(r)
		isOpen, err := exchange.ToggleExchange(token)
		if err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, map[string]bool{"isOpen": isOpen}, 200)
	})

	mux.HandleFunc("/api/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		token := getToken(r)
		body := readBody(r)
		targetID := getString(body, "targetUserId")
		if targetID != "" {
			if err := exchange.ResetUser(token, targetID); err != nil {
				errResp(w, err.Error(), 400); return
			}
		} else {
			if err := exchange.ResetAll(token); err != nil {
				errResp(w, err.Error(), 400); return
			}
		}
		jsonResp(w, map[string]bool{"success": true}, 200)
	})

	mux.HandleFunc("/api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		token := getToken(r)
		users, err := exchange.GetAllUsers(token)
		if err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, users, 200)
	})

	mux.HandleFunc("/api/admin/no-pos-limit", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }
		token := getToken(r)
		body := readBody(r)
		targetID := getString(body, "targetUserId")
		value := true
		if v, ok := body["value"]; ok {
			if b, ok := v.(bool); ok { value = b }
		}
		if err := exchange.SetNoPositionLimit(token, targetID, value); err != nil {
			errResp(w, err.Error(), 400); return
		}
		jsonResp(w, map[string]bool{"success": true}, 200)
	})

	mux.HandleFunc("/api/admin/rate-limit", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }
		token := getToken(r)
		body := readBody(r)
		targetID := getString(body, "targetUserId")
		ms := getInt(body, "rateLimitMs")
		if err := exchange.SetRateLimit(token, targetID, ms); err != nil {
			errResp(w, err.Error(), 400); return
		}
		jsonResp(w, map[string]bool{"success": true}, 200)
	})

	mux.HandleFunc("/api/admin/config", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }
		cfg, err := exchange.UpdateConfig(getToken(r), readBody(r))
		if err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, cfg, 200)
	})

	mux.HandleFunc("/api/admin/end-day", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }
		if err := exchange.EndDayNow(getToken(r)); err != nil { errResp(w, err.Error(), 400); return }
		jsonResp(w, map[string]bool{"success": true}, 200)
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		isAdmin := false
		if u, err := exchange.AuthenticateR(getToken(r)); err == nil { isAdmin = u.IsAdmin }
		jsonResp(w, exchange.GetConfig(isAdmin), 200)
	})

	mux.HandleFunc("/api/option-windows", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		expiry, limit := 0, 100
		fmt.Sscan(r.URL.Query().Get("expiry"), &expiry)
		fmt.Sscan(r.URL.Query().Get("limit"), &limit)
		if limit <= 0 || limit > 5000 { limit = 100 }
		jsonResp(w, exchange.GetOptionWindows(expiry, limit), 200)
	})

	mux.HandleFunc("/api/daily-results", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetDailyResults(), 200)
	})

	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.rec.Files(), 200)
	})

	mux.HandleFunc("/api/data/", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		path, err := exchange.rec.Resolve(strings.TrimPrefix(r.URL.Path, "/api/data/"))
		if err != nil { errResp(w, err.Error(), 404); return }
		w.Header().Set("Content-Type", "application/vnd.apache.parquet")
		http.ServeFile(w, r, path)
	})

	// Public query endpoints
	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetAllProducts(), 200)
	})

	mux.HandleFunc("/api/exchange-state", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetExchangeState(), 200)
	})

	mux.HandleFunc("/api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetLeaderboard(), 200)
	})

	mux.HandleFunc("/api/pnl-history", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetPnlHistory(), 200)
	})

	mux.HandleFunc("/api/price-history", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetPriceHistory(), 200)
	})

	mux.HandleFunc("/api/all-positions", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetAllPositions(), 200)
	})

	mux.HandleFunc("/api/volume-matrix", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		jsonResp(w, exchange.GetVolumeMatrix(), 200)
	})

	mux.HandleFunc("/api/trades", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		token := getToken(r)
		userID := ""
		if token != "" {
			if u, err := exchange.AuthenticateR(token); err == nil {
				userID = u.ID
			}
		}
		jsonResp(w, exchange.GetRecentTrades(userID), 200)
	})

	port := ":3211"
	log.Printf("Exchange server starting on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatal(err)
	}
}

func resolveProduct(symbol string) string {
	exchange.mu.RLock()
	defer exchange.mu.RUnlock()
	return exchange.Symbols[symbol]
}
