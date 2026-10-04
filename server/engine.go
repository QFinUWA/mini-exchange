package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// --- Types ---

type User struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	PasswordHash    string `json:"passwordHash"`
	Salt            string `json:"salt"`
	IsAdmin         bool   `json:"isAdmin"`
	NoPositionLimit bool   `json:"noPositionLimit,omitempty"`
	RateLimitMs     int    `json:"rateLimitMs,omitempty"`
	SessionToken    string `json:"sessionToken"`
	IsHouse         bool   `json:"isHouse,omitempty"`
	lastOrderAt     int64
}

type Product struct {
	ID              string            `json:"id"`
	Symbol          string            `json:"symbol"`
	Name            string            `json:"name"`
	IsEtf           bool              `json:"isEtf"`
	EtfComposition  []EtfComponent    `json:"etfComposition,omitempty"`
	EtfCompositions [][]EtfComponent  `json:"etfCompositions,omitempty"`
	PositionLimit   int               `json:"positionLimit"`
	TickSize        float64           `json:"tickSize"`
	IsActive        bool              `json:"isActive"`
	Kind            string            `json:"kind,omitempty"`
	ExpiryMin       int               `json:"expiryMin,omitempty"`
}

func (p *Product) getCompositions() [][]EtfComponent {
	if len(p.EtfCompositions) > 0 {
		return p.EtfCompositions
	}
	if len(p.EtfComposition) > 0 {
		return [][]EtfComponent{p.EtfComposition}
	}
	return nil
}

type EtfComponent struct {
	ProductID string  `json:"productId"`
	Weight    float64 `json:"weight"`
}

type Order struct {
	ID        string  `json:"id"`
	UserID    string  `json:"userId"`
	ProductID string  `json:"productId"`
	Side      string  `json:"side"`
	Price     float64 `json:"price"`
	Qty       int     `json:"qty"`
	FilledQty int     `json:"filledQty"`
	OrderType string  `json:"orderType"`
	CreatedAt int64   `json:"createdAt"`
}

func (o *Order) Remaining() int { return o.Qty - o.FilledQty }

type Position struct {
	UserID        string  `json:"userId"`
	ProductID     string  `json:"productId"`
	Qty           int     `json:"qty"`
	AvgEntryPrice float64 `json:"avgEntryPrice"`
	RealizedPnl   float64 `json:"realizedPnl"`
}

type Fill struct {
	ID         string  `json:"id"`
	ProductID  string  `json:"productId"`
	BuyUserID  string  `json:"buyUserId"`
	SellUserID string  `json:"sellUserId"`
	Price      float64 `json:"price"`
	Qty        int     `json:"qty"`
	CreatedAt  int64   `json:"createdAt"`
}

type PriceSnapshot struct {
	Symbol     string  `json:"symbol"`
	Mid        float64 `json:"mid"`
	SnapshotAt int64   `json:"snapshotAt"`
}

type PnlSnapshot struct {
	UserID      string  `json:"userId"`
	TotalPnl    float64 `json:"totalPnl"`
	RealizedPnl float64 `json:"realizedPnl"`
	SnapshotAt  int64   `json:"snapshotAt"`
}

type OrderResult struct {
	OrderID string     `json:"orderId"`
	Fills   []FillInfo `json:"fills"`
	Status  string     `json:"status"`
}

type FillInfo struct {
	Price float64 `json:"price"`
	Qty   int     `json:"qty"`
}

// --- API Response Types ---

type BookLevel struct {
	Price      float64 `json:"price"`
	Qty        int     `json:"qty"`
	OrderCount int     `json:"orderCount"`
}

type BookSnapshot struct {
	Bids           []BookLevel `json:"bids"`
	Asks           []BookLevel `json:"asks"`
	LastTradePrice *float64    `json:"lastTradePrice"`
}

type PositionInfo struct {
	ProductID     string  `json:"productId"`
	Symbol        string  `json:"symbol"`
	Qty           int     `json:"qty"`
	RealizedPnl   float64 `json:"realizedPnl"`
	AvgEntryPrice float64 `json:"avgEntryPrice"`
	UnrealizedPnl float64 `json:"unrealizedPnl"`
}

type UserOrderInfo struct {
	ID        string  `json:"_id"`
	ProductID string  `json:"productId"`
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Price     float64 `json:"price"`
	Qty       int     `json:"qty"`
	FilledQty int     `json:"filledQty"`
	Status    string  `json:"status"`
	OrderType string  `json:"orderType"`
	CreatedAt int64   `json:"createdAt"`
}

type TradeInfo struct {
	ID        string  `json:"_id"`
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Qty       int     `json:"qty"`
	CreatedAt int64   `json:"createdAt"`
	IsMine    bool    `json:"isMine"`
	MySide    *string `json:"mySide"`
	Buyer     string  `json:"buyer"`
	Seller    string  `json:"seller"`
}

type LeaderboardEntry struct {
	UserID        string  `json:"userId"`
	Username      string  `json:"username"`
	TotalPnl      float64 `json:"totalPnl"`
	RealizedPnl   float64 `json:"realizedPnl"`
	UnrealizedPnl float64 `json:"unrealizedPnl"`
	Days          int     `json:"days"`
	MeanDailyPnl  float64 `json:"meanDailyPnl"`
	StdDailyPnl   float64 `json:"stdDailyPnl"`
	Score         float64 `json:"score"`
	Busts         int     `json:"busts"`
}

type BotState struct {
	ExchangeOpen  bool                    `json:"exchangeOpen"`
	ServerTime    int64                   `json:"serverTime"`
	Products      []BotProduct            `json:"products"`
	Books         map[string]BookSnapshot `json:"books"`
	Positions     []PositionInfo          `json:"positions"`
	OpenOrders    []UserOrderInfo         `json:"openOrders"`
	Pnl           float64                 `json:"pnl"`
	Account       AccountInfo             `json:"account"`
	Config        PublicConfig            `json:"config"`
	OptionWindows []OptionWindow          `json:"optionWindows"`
}

type BotProduct struct {
	ID            string             `json:"id"`
	Symbol        string             `json:"symbol"`
	Name          string             `json:"name"`
	IsEtf         bool               `json:"isEtf"`
	Kind          string             `json:"kind"`
	Compositions  [][]BotComposition `json:"compositions,omitempty"`
	PositionLimit int                `json:"positionLimit"`
	TickSize      float64            `json:"tickSize"`
	Option        *OptionInfo        `json:"option,omitempty"`
}

type BotComposition struct {
	Symbol string  `json:"symbol"`
	Weight float64 `json:"weight"`
}

// --- Exchange ---

type Exchange struct {
	mu sync.RWMutex

	Users      map[string]*User      `json:"users"`
	Sessions   map[string]string     `json:"sessions"`
	Usernames  map[string]string     `json:"usernames"`
	Products   map[string]*Product   `json:"products"`
	Symbols    map[string]string     `json:"symbols"`
	Orders     map[string]*Order     `json:"orders"`
	Positions  map[string]*Position  `json:"positions"`
	Fills      []*Fill               `json:"fills"`
	PnlSnaps    []PnlSnapshot         `json:"pnlSnaps"`
	PriceSnaps  []PriceSnapshot      `json:"priceSnaps"`
	IsOpen      bool                 `json:"isOpen"`

	Config        *Config               `json:"config"`
	Accounts      map[string]*Account   `json:"accounts"`
	DailyResults  []DailyResult         `json:"dailyResults"`
	Options       map[int]*OptionState  `json:"options"`
	OptionHistory []OptionWindow        `json:"optionHistory"`
	LastPrice     map[string]float64    `json:"lastPrice"`
	EtfTradeLog   []pricePoint          `json:"etfTradeLog"`
	EtfMidLog     []pricePoint          `json:"etfMidLog"`
	DayStart      int64                 `json:"dayStart"`
	Sim           *SimState             `json:"sim"`

	rec *Recorder

	nextID atomic.Int64

	broadcast      func()
	savePath       string
	dirty          atomic.Bool
	broadcastPending atomic.Bool
}

func NewExchange(savePath string) *Exchange {
	ex := &Exchange{
		Users:     make(map[string]*User),
		Sessions:  make(map[string]string),
		Usernames: make(map[string]string),
		Products:  make(map[string]*Product),
		Symbols:   make(map[string]string),
		Orders:    make(map[string]*Order),
		Positions: make(map[string]*Position),
		savePath:  savePath,
	}
	ex.nextID.Store(1)
	return ex
}

func (ex *Exchange) genID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, ex.nextID.Add(1)-1)
}

func posKey(userID, productID string) string {
	return userID + ":" + productID
}

func hashPassword(password, salt string) string {
	data := []byte(salt + password)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// --- Auth ---

func (ex *Exchange) Register(username, password string) (*User, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	if len(username) < 1 || len(username) > 20 {
		return nil, fmt.Errorf("username must be 1-20 chars")
	}
	if _, ok := ex.Usernames[username]; ok {
		return nil, fmt.Errorf("username already taken")
	}

	salt := randomHex(16)
	isAdmin := true
	for _, other := range ex.Users {
		if !other.IsHouse { isAdmin = false; break }
	}
	token := randomHex(16)

	u := &User{
		ID:           ex.genID("u"),
		Username:     username,
		PasswordHash: hashPassword(password, salt),
		Salt:         salt,
		IsAdmin:      isAdmin,
		SessionToken: token,
	}
	ex.Users[u.ID] = u
	ex.Sessions[token] = u.ID
	ex.Usernames[username] = u.ID
	ex.account(u.ID)

	ex.saveAsync()
	return u, nil
}

func (ex *Exchange) Login(username, password string) (*User, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	uid, ok := ex.Usernames[username]
	if !ok {
		return nil, fmt.Errorf("invalid credentials")
	}
	u := ex.Users[uid]
	if u.IsHouse || hashPassword(password, u.Salt) != u.PasswordHash {
		return nil, fmt.Errorf("invalid credentials")
	}
	if u.SessionToken == "" {
		u.SessionToken = randomHex(16)
		ex.Sessions[u.SessionToken] = u.ID
	}
	ex.saveAsync()
	return u, nil
}

func (ex *Exchange) Logout(token string) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if uid, ok := ex.Sessions[token]; ok {
		if u, ok := ex.Users[uid]; ok {
			u.SessionToken = ""
		}
		delete(ex.Sessions, token)
	}
}

func (ex *Exchange) Authenticate(token string) (*User, error) {
	uid, ok := ex.Sessions[token]
	if !ok {
		return nil, fmt.Errorf("invalid session")
	}
	u, ok := ex.Users[uid]
	if !ok {
		return nil, fmt.Errorf("invalid session")
	}
	return u, nil
}

func (ex *Exchange) AuthenticateR(token string) (*User, error) {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.Authenticate(token)
}

// --- Admin ---

func (ex *Exchange) CreateProduct(token, symbol, name string, posLimit int) (*Product, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil { return nil, err }
	if !u.IsAdmin { return nil, fmt.Errorf("admin required") }
	if _, ok := ex.Symbols[symbol]; ok { return nil, fmt.Errorf("symbol already taken") }

	p := &Product{
		ID: ex.genID("p"), Symbol: symbol, Name: name,
		PositionLimit: posLimit, TickSize: 1, IsActive: true,
	}
	ex.Products[p.ID] = p
	ex.Symbols[symbol] = p.ID
	ex.saveAsync()
	ex.doBroadcast()
	return p, nil
}

func (ex *Exchange) checkEtfCycle(newID string, compositions [][]EtfComponent) error {
	visited := map[string]bool{newID: true}
	var walk func(comps []EtfComponent) error
	walk = func(comps []EtfComponent) error {
		for _, c := range comps {
			if visited[c.ProductID] {
				return fmt.Errorf("circular ETF dependency detected")
			}
			cp := ex.Products[c.ProductID]
			if cp != nil && cp.IsEtf {
				visited[c.ProductID] = true
				for _, subComp := range cp.getCompositions() {
					if err := walk(subComp); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	for _, comp := range compositions {
		if err := walk(comp); err != nil {
			return err
		}
	}
	return nil
}

func (ex *Exchange) CreateEtf(token, symbol, name string, posLimit int, compositions [][]EtfComponent) (*Product, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil { return nil, err }
	if !u.IsAdmin { return nil, fmt.Errorf("admin required") }
	if _, ok := ex.Symbols[symbol]; ok { return nil, fmt.Errorf("symbol already taken") }
	if len(compositions) == 0 { return nil, fmt.Errorf("at least one composition required") }
	for _, comp := range compositions {
		if len(comp) == 0 { return nil, fmt.Errorf("each composition must have at least one component") }
		for _, c := range comp {
			cp, ok := ex.Products[c.ProductID]
			if !ok {
				return nil, fmt.Errorf("component product not found: %s", c.ProductID)
			}
			if c.Weight <= 0 {
				return nil, fmt.Errorf("weight must be positive for %s", cp.Symbol)
			}
		}
	}

	newID := ex.genID("p")
	if err := ex.checkEtfCycle(newID, compositions); err != nil {
		return nil, err
	}

	p := &Product{
		ID: newID, Symbol: symbol, Name: name,
		IsEtf: true, EtfCompositions: compositions,
		PositionLimit: posLimit, TickSize: 1, IsActive: true,
	}
	ex.Products[p.ID] = p
	ex.Symbols[symbol] = p.ID
	ex.saveAsync()
	ex.doBroadcast()
	return p, nil
}

func (ex *Exchange) DeleteProduct(token, productID string) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }
	p, ok := ex.Products[productID]
	if !ok { return fmt.Errorf("product not found") }
	p.IsActive = false
	delete(ex.Symbols, p.Symbol)
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) UpdatePositionLimit(token, productID string, limit int) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }
	p, ok := ex.Products[productID]
	if !ok { return fmt.Errorf("product not found") }
	p.PositionLimit = limit
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) UpdateTickSize(token, productID string, tickSize float64) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }
	if tickSize <= 0 { return fmt.Errorf("tick size must be positive") }
	p, ok := ex.Products[productID]
	if !ok { return fmt.Errorf("product not found") }
	p.TickSize = tickSize
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) ToggleExchange(token string) (bool, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return false, err }
	if !u.IsAdmin { return false, fmt.Errorf("admin required") }
	ex.IsOpen = !ex.IsOpen
	ex.saveAsync()
	ex.doBroadcast()
	return ex.IsOpen, nil
}

func (ex *Exchange) ResetAll(token string) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }

	for id, user := range ex.Users {
		if id == u.ID || user.IsHouse { continue }
		delete(ex.Users, id)
		delete(ex.Usernames, user.Username)
		delete(ex.Sessions, user.SessionToken)
	}

	ex.Products = make(map[string]*Product)
	ex.Symbols = make(map[string]string)
	ex.Orders = make(map[string]*Order)
	ex.Positions = make(map[string]*Position)
	ex.Fills = nil
	ex.PnlSnaps = nil
	ex.PriceSnaps = nil
	ex.IsOpen = false
	ex.Accounts = nil
	ex.DailyResults = nil
	ex.Options = nil
	ex.OptionHistory = nil
	ex.LastPrice = nil
	ex.EtfTradeLog = nil
	ex.EtfMidLog = nil
	ex.Sim = nil
	ex.DayStart = 0
	ex.ensureProject()
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) ResetUser(token, targetUserID string) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }

	for id, o := range ex.Orders {
		if o.UserID == targetUserID { delete(ex.Orders, id) }
	}
	for k, p := range ex.Positions {
		if p.UserID == targetUserID { delete(ex.Positions, k) }
	}
	filtered := ex.Fills[:0]
	for _, f := range ex.Fills {
		if f.BuyUserID != targetUserID && f.SellUserID != targetUserID {
			filtered = append(filtered, f)
		}
	}
	ex.Fills = filtered
	filteredSnaps := ex.PnlSnaps[:0]
	for _, s := range ex.PnlSnaps {
		if s.UserID != targetUserID { filteredSnaps = append(filteredSnaps, s) }
	}
	ex.PnlSnaps = filteredSnaps
	ex.resetUserDay(targetUserID)
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) GetAllUsers(token string) ([]map[string]interface{}, error) {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	u, err := ex.Authenticate(token)
	if err != nil { return nil, err }
	if !u.IsAdmin { return nil, fmt.Errorf("admin required") }
	return ex.userList(), nil
}

func (ex *Exchange) SetNoPositionLimit(token, targetUserID string, value bool) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }
	target, ok := ex.Users[targetUserID]
	if !ok { return fmt.Errorf("user not found") }
	target.NoPositionLimit = value
	ex.saveAsync()
	return nil
}

func (ex *Exchange) SetRateLimit(token, targetUserID string, ms int) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }
	target, ok := ex.Users[targetUserID]
	if !ok { return fmt.Errorf("user not found") }
	target.RateLimitMs = ms
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) checkRateLimit(u *User) error {
	limit := u.RateLimitMs
	if limit <= 0 { limit = 10 }
	now := time.Now().UnixMilli()
	if now-u.lastOrderAt < int64(limit) {
		return fmt.Errorf("rate limited (%dms cooldown)", limit)
	}
	u.lastOrderAt = now
	return nil
}

// --- Trading ---

func (ex *Exchange) PlaceOrder(token, productID, side string, price float64, qty int, orderType string) (*OrderResult, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil { return nil, err }
	if !ex.IsOpen { return nil, fmt.Errorf("exchange is closed") }
	if err := ex.checkRateLimit(u); err != nil { return nil, err }

	result, err := ex.placeOrderInternal(u.ID, productID, side, price, qty, orderType)
	if err != nil { return nil, err }
	ex.saveAsync()
	ex.doBroadcast()
	return result, nil
}

func (ex *Exchange) matchOrder(order *Order) []*Fill {
	if order.OrderType == "postOnly" {
		if ex.wouldMatch(order) {
			order.Qty = 0
			return nil
		}
		return nil
	}

	oppSide := "sell"
	if order.Side == "sell" { oppSide = "buy" }

	resting := ex.getBookSide(order.ProductID, oppSide)

	if order.Side == "buy" {
		sort.Slice(resting, func(i, j int) bool {
			if resting[i].Price != resting[j].Price { return resting[i].Price < resting[j].Price }
			return resting[i].CreatedAt < resting[j].CreatedAt
		})
	} else {
		sort.Slice(resting, func(i, j int) bool {
			if resting[i].Price != resting[j].Price { return resting[i].Price > resting[j].Price }
			return resting[i].CreatedAt < resting[j].CreatedAt
		})
	}

	var fills []*Fill
	for _, rest := range resting {
		if order.Remaining() == 0 { break }

		matchable := false
		if order.Side == "buy" { matchable = rest.Price <= order.Price }
		if order.Side == "sell" { matchable = rest.Price >= order.Price }
		if !matchable { break }

		if rest.UserID == order.UserID {
			cancelQty := min(order.Remaining(), rest.Remaining())
			newRestQty := rest.Qty - cancelQty
			if newRestQty == rest.FilledQty {
				delete(ex.Orders, rest.ID)
			} else {
				rest.Qty = newRestQty
			}
			order.Qty -= cancelQty
			continue
		}

		fillQty := min(order.Remaining(), rest.Remaining())
		fillPrice := rest.Price
		now := time.Now().UnixMilli()

		buyUID := order.UserID
		sellUID := rest.UserID
		if order.Side == "sell" { buyUID, sellUID = sellUID, buyUID }

		fill := &Fill{
			ID: ex.genID("f"), ProductID: order.ProductID,
			BuyUserID: buyUID, SellUserID: sellUID,
			Price: fillPrice, Qty: fillQty, CreatedAt: now,
		}
		ex.Fills = append(ex.Fills, fill)
		if len(ex.Fills) > 500 { ex.Fills = ex.Fills[len(ex.Fills)-500:] }
		ex.LastPrice[order.ProductID] = fillPrice
		if !ex.isHouse(buyUID) { ex.account(buyUID).Active = true }
		if !ex.isHouse(sellUID) { ex.account(sellUID).Active = true }

		rest.FilledQty += fillQty
		if rest.Remaining() == 0 {
			delete(ex.Orders, rest.ID)
		}

		order.FilledQty += fillQty

		ex.updatePosition(order.UserID, order.ProductID, fillQty, fillPrice, order.Side)
		ex.updatePosition(rest.UserID, order.ProductID, fillQty, fillPrice, oppSide)

		fills = append(fills, fill)
	}

	return fills
}

func (ex *Exchange) wouldMatch(order *Order) bool {
	oppSide := "sell"
	if order.Side == "sell" { oppSide = "buy" }
	for _, o := range ex.Orders {
		if o.ProductID != order.ProductID || o.Side != oppSide { continue }
		if o.UserID == order.UserID { continue }
		if order.Side == "buy" && o.Price <= order.Price { return true }
		if order.Side == "sell" && o.Price >= order.Price { return true }
	}
	return false
}

func (ex *Exchange) CancelOrder(token, orderID string) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil { return err }

	o, ok := ex.Orders[orderID]
	if !ok { return fmt.Errorf("order not found") }
	if o.UserID != u.ID { return fmt.Errorf("not your order") }

	delete(ex.Orders, orderID)
	ex.saveAsync()
	ex.doBroadcast()
	return nil
}

func (ex *Exchange) CancelAll(token string, productID *string, side *string) (int, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil { return 0, err }

	count := 0
	for id, o := range ex.Orders {
		if o.UserID != u.ID { continue }
		if productID != nil && o.ProductID != *productID { continue }
		if side != nil && o.Side != *side { continue }
		delete(ex.Orders, id)
		count++
	}
	ex.saveAsync()
	ex.doBroadcast()
	return count, nil
}

// --- Batch ---

type BatchAction struct {
	Type      string   `json:"type"`
	ProductID string   `json:"productId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	Side      string   `json:"side,omitempty"`
	Price     float64  `json:"price,omitempty"`
	Qty       float64  `json:"qty,omitempty"`
	OrderType string   `json:"orderType,omitempty"`
	OrderID   string   `json:"orderId,omitempty"`
	Direction        string    `json:"direction,omitempty"`
	CompositionIndex int       `json:"compositionIndex,omitempty"`
	Prices           []float64 `json:"prices,omitempty"`
}

type BatchResult struct {
	OK    bool                   `json:"ok"`
	Data  map[string]interface{} `json:"data,omitempty"`
	Error string                 `json:"error,omitempty"`
}

func (ex *Exchange) ExecuteBatch(token string, actions []BatchAction) []BatchResult {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil {
		return []BatchResult{{OK: false, Error: err.Error()}}
	}
	if !ex.IsOpen {
		return []BatchResult{{OK: false, Error: "exchange is closed"}}
	}

	if err := ex.checkRateLimit(u); err != nil {
		return []BatchResult{{OK: false, Error: err.Error()}}
	}

	results := make([]BatchResult, len(actions))
	for i, a := range actions {
		switch a.Type {
		case "order":
			r, err := ex.placeOrderInternal(u.ID, a.ProductID, a.Side, a.Price, int(a.Qty), a.OrderType)
			if err != nil {
				results[i] = BatchResult{OK: false, Error: err.Error()}
			} else {
				results[i] = BatchResult{OK: true, Data: map[string]interface{}{
					"orderId": r.OrderID, "fills": r.Fills, "status": r.Status,
				}}
			}
		case "cancel":
			o, ok := ex.Orders[a.OrderID]
			if !ok {
				results[i] = BatchResult{OK: false, Error: "order not found"}
			} else if o.UserID != u.ID {
				results[i] = BatchResult{OK: false, Error: "not your order"}
			} else {
				delete(ex.Orders, a.OrderID)
				results[i] = BatchResult{OK: true}
			}
		case "cancelAll":
			count := 0
			for id, o := range ex.Orders {
				if o.UserID != u.ID { continue }
				if a.ProductID != "" && o.ProductID != a.ProductID { continue }
				if a.Side != "" && o.Side != a.Side { continue }
				delete(ex.Orders, id)
				count++
			}
			results[i] = BatchResult{OK: true, Data: map[string]interface{}{"cancelled": count}}
		case "etfSwap":
			r, err := ex.swapEtfInternal(u.ID, a.ProductID, int(a.Qty), a.Direction, a.CompositionIndex, a.Prices)
			if err != nil {
				results[i] = BatchResult{OK: false, Error: err.Error()}
			} else {
				results[i] = BatchResult{OK: true, Data: r}
			}
		default:
			results[i] = BatchResult{OK: false, Error: "unknown action type"}
		}
	}

	ex.saveAsync()
	ex.doBroadcast()
	return results
}

func (ex *Exchange) placeOrderInternal(userID, productID, side string, price float64, qty int, orderType string) (*OrderResult, error) {
	if qty <= 0 { return nil, fmt.Errorf("qty must be positive") }
	if price <= 0 { return nil, fmt.Errorf("price must be positive") }
	if side != "buy" && side != "sell" { return nil, fmt.Errorf("side must be buy or sell") }

	p, ok := ex.Products[productID]
	if !ok { return nil, fmt.Errorf("product not found") }
	if !p.IsActive { return nil, fmt.Errorf("product is not active") }

	ticks := math.Round(price / p.TickSize)
	if math.Abs(price-ticks*p.TickSize) > 1e-6 {
		return nil, fmt.Errorf("price must be a multiple of tick size %v", p.TickSize)
	}
	price = ticks * p.TickSize
	if p.TickSize < 1 { price = math.Round(price*1e8) / 1e8 }
	if p.kind() == KindOption && (price < 0.01-1e-9 || price > 0.99+1e-9) {
		return nil, fmt.Errorf("option price must be between 0.01 and 0.99")
	}

	u := ex.Users[userID]
	house := u != nil && u.IsHouse
	if u == nil || !u.NoPositionLimit {
		pos := ex.getPosition(userID, productID)
		sameSideResting := 0
		if orderType != "ioc" {
			sameSideResting = ex.restingQty(userID, productID, side)
		}
		var maxQty int
		if side == "buy" {
			maxQty = p.PositionLimit - pos.Qty - sameSideResting
		} else {
			maxQty = p.PositionLimit + pos.Qty - sameSideResting
		}
		if maxQty <= 0 {
			return nil, fmt.Errorf("position limit (%d) reached", p.PositionLimit)
		}
		if qty > maxQty { qty = maxQty }
	}

	if !house {
		marks := ex.markPrices()
		req := ex.marginRequired(userID, marks, &hypoOrder{productID, side, price, qty})
		eq := ex.equity(userID, marks)
		if req > eq {
			return nil, fmt.Errorf("insufficient margin (required %.2f, equity %.2f)", req, eq)
		}
	}

	order := &Order{
		ID: ex.genID("o"), UserID: userID, ProductID: productID,
		Side: side, Price: price, Qty: qty, FilledQty: 0,
		OrderType: orderType, CreatedAt: time.Now().UnixMilli(),
	}

	fills := ex.matchOrder(order)
	filledQty := order.FilledQty
	var status string
	if order.Qty == 0 {
		status = "cancelled"
	} else if filledQty == order.Qty {
		status = "filled"
	} else if orderType == "ioc" {
		status = "cancelled"
	} else if filledQty > 0 {
		status = "partial"
		ex.Orders[order.ID] = order
	} else {
		status = "open"
		ex.Orders[order.ID] = order
	}

	var fillInfos []FillInfo
	notional := 0.0
	for _, f := range fills {
		fillInfos = append(fillInfos, FillInfo{Price: f.Price, Qty: f.Qty})
		notional += f.Price * float64(f.Qty)
	}
	if filledQty > 0 {
		ex.onAggressiveTrade(p, side, notional/float64(filledQty), filledQty, order.CreatedAt)
	}
	return &OrderResult{OrderID: order.ID, Fills: fillInfos, Status: status}, nil
}

// onAggressiveTrade records one row per aggressing order (average fill price), as in the data.
func (ex *Exchange) onAggressiveTrade(p *Product, side string, avgPrice float64, qty int, atMs int64) {
	sec := atMs / 1000
	if p.kind() == KindEtf { ex.recordEtfTrade(sec, avgPrice) }
	if ex.rec == nil { return }
	if p.kind() == KindOption {
		if os := ex.optionStateFor(p.ID); os != nil {
			ex.rec.addOptionTrade(sec, os.ExpiryMin, os.WindowOpen, avgPrice, qty, side+"_up")
		}
		return
	}
	ex.rec.addTrade(sec, p.Symbol, avgPrice, qty, side)
}

// --- Positions ---

func (ex *Exchange) getPosition(userID, productID string) *Position {
	k := posKey(userID, productID)
	pos, ok := ex.Positions[k]
	if !ok {
		pos = &Position{UserID: userID, ProductID: productID}
		ex.Positions[k] = pos
	}
	return pos
}

func (ex *Exchange) updatePosition(userID, productID string, fillQty int, fillPrice float64, side string) {
	pos := ex.getPosition(userID, productID)
	fq := float64(fillQty)

	if side == "buy" {
		ex.account(userID).Cash -= fillPrice * fq
	} else {
		ex.account(userID).Cash += fillPrice * fq
	}

	if side == "buy" {
		if pos.Qty >= 0 {
			newQty := pos.Qty + fillQty
			if newQty == 0 {
				pos.AvgEntryPrice = 0
			} else {
				pos.AvgEntryPrice = (pos.AvgEntryPrice*float64(pos.Qty) + fillPrice*fq) / float64(newQty)
			}
			pos.Qty = newQty
		} else {
			closeQty := min(fillQty, -pos.Qty)
			realized := float64(closeQty) * (pos.AvgEntryPrice - fillPrice)
			remainQty := pos.Qty + fillQty
			if remainQty > 0 {
				pos.AvgEntryPrice = fillPrice
			} else if remainQty == 0 {
				pos.AvgEntryPrice = 0
			}
			pos.Qty = remainQty
			pos.RealizedPnl += realized
		}
	} else {
		if pos.Qty <= 0 {
			newQty := pos.Qty - fillQty
			absNew := -newQty
			if absNew == 0 {
				pos.AvgEntryPrice = 0
			} else {
				pos.AvgEntryPrice = (pos.AvgEntryPrice*float64(-pos.Qty) + fillPrice*fq) / float64(absNew)
			}
			pos.Qty = newQty
		} else {
			closeQty := min(fillQty, pos.Qty)
			realized := float64(closeQty) * (fillPrice - pos.AvgEntryPrice)
			remainQty := pos.Qty - fillQty
			if remainQty < 0 {
				pos.AvgEntryPrice = fillPrice
			} else if remainQty == 0 {
				pos.AvgEntryPrice = 0
			}
			pos.Qty = remainQty
			pos.RealizedPnl += realized
		}
	}
}

func (ex *Exchange) restingQty(userID, productID, side string) int {
	total := 0
	for _, o := range ex.Orders {
		if o.UserID == userID && o.ProductID == productID && o.Side == side {
			total += o.Remaining()
		}
	}
	return total
}

// --- ETF ---

func (ex *Exchange) SwapEtf(token, productID string, qty int, direction string, compIdx int, prices []float64) (map[string]interface{}, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	u, err := ex.Authenticate(token)
	if err != nil { return nil, err }
	if !ex.IsOpen { return nil, fmt.Errorf("exchange is closed") }

	result, err := ex.swapEtfInternal(u.ID, productID, qty, direction, compIdx, prices)
	if err != nil { return nil, err }
	ex.saveAsync()
	ex.doBroadcast()
	return result, nil
}

func (ex *Exchange) swapEtfInternal(userID, productID string, qty int, direction string, compIdx int, prices []float64) (map[string]interface{}, error) {
	if qty <= 0 { return nil, fmt.Errorf("qty must be positive") }

	p, ok := ex.Products[productID]
	if !ok { return nil, fmt.Errorf("product not found") }
	allComps := p.getCompositions()
	if !p.IsEtf || len(allComps) == 0 { return nil, fmt.Errorf("not an ETF") }
	if compIdx < 0 || compIdx >= len(allComps) { return nil, fmt.Errorf("invalid composition index") }

	composition := allComps[compIdx]

	if prices != nil && len(prices) != len(composition) {
		return nil, fmt.Errorf("prices length must match composition")
	}

	type compInfo struct {
		productID string
		weight    float64
		mid       float64
		posLimit  int
	}

	var comps []compInfo
	fairValue := 0.0
	for i, c := range composition {
		cp, ok := ex.Products[c.ProductID]
		if !ok { return nil, fmt.Errorf("component not found") }

		var mid float64
		if prices != nil {
			mid = prices[i]
			if mid <= 0 { return nil, fmt.Errorf("invalid price for %s", cp.Symbol) }
		} else {
			midP := ex.getMidPrice(c.ProductID)
			if midP == nil { return nil, fmt.Errorf("no mid price for %s", cp.Symbol) }
			mid = *midP
		}
		comps = append(comps, compInfo{c.ProductID, c.Weight, mid, cp.PositionLimit})
		fairValue += c.Weight * mid
	}

	if direction == "create" {
		for _, c := range comps {
			pos := ex.getPosition(userID, c.productID)
			maxForComp := int(math.Floor(float64(pos.Qty) / c.weight))
			if maxForComp < qty { qty = maxForComp }
		}
		etfPos := ex.getPosition(userID, productID)
		maxEtf := p.PositionLimit - etfPos.Qty
		if maxEtf < qty { qty = maxEtf }
		if qty <= 0 {
			return map[string]interface{}{"success": true, "fairValue": fairValue, "qty": 0, "direction": direction, "fee": 0.0}, nil
		}
		for _, c := range comps {
			legQty := int(math.Round(c.weight * float64(qty)))
			ex.updatePosition(userID, c.productID, legQty, c.mid, "sell")
		}
		ex.updatePosition(userID, productID, qty, fairValue, "buy")
	} else {
		etfPos := ex.getPosition(userID, productID)
		if etfPos.Qty < qty { qty = etfPos.Qty }
		for _, c := range comps {
			pos := ex.getPosition(userID, c.productID)
			maxForComp := int(math.Floor(float64(c.posLimit-pos.Qty) / c.weight))
			if maxForComp < qty { qty = maxForComp }
		}
		if qty <= 0 {
			return map[string]interface{}{"success": true, "fairValue": fairValue, "qty": 0, "direction": direction, "fee": 0.0}, nil
		}
		ex.updatePosition(userID, productID, qty, fairValue, "sell")
		for _, c := range comps {
			legQty := int(math.Round(c.weight * float64(qty)))
			ex.updatePosition(userID, c.productID, legQty, c.mid, "buy")
		}
	}

	fee := 0.0
	if !ex.isHouse(userID) {
		fee = round2(ex.Config.EtfFee * float64(qty))
		a := ex.account(userID)
		a.Cash -= fee
		a.Fees += fee
		a.Active = true
	}
	return map[string]interface{}{"success": true, "fairValue": fairValue, "qty": qty, "direction": direction, "fee": fee}, nil
}

// --- Queries ---

func (ex *Exchange) getBookSide(productID, side string) []*Order {
	var orders []*Order
	for _, o := range ex.Orders {
		if o.ProductID == productID && o.Side == side {
			orders = append(orders, o)
		}
	}
	return orders
}

func (ex *Exchange) getBookSnapshot(productID string) BookSnapshot {
	bidMap := make(map[float64]*BookLevel)
	askMap := make(map[float64]*BookLevel)
	for _, o := range ex.Orders {
		if o.ProductID != productID { continue }
		rem := o.Remaining()
		if rem <= 0 { continue }
		if o.Side == "buy" {
			if l, ok := bidMap[o.Price]; ok {
				l.Qty += rem; l.OrderCount++
			} else {
				bidMap[o.Price] = &BookLevel{Price: o.Price, Qty: rem, OrderCount: 1}
			}
		} else {
			if l, ok := askMap[o.Price]; ok {
				l.Qty += rem; l.OrderCount++
			} else {
				askMap[o.Price] = &BookLevel{Price: o.Price, Qty: rem, OrderCount: 1}
			}
		}
	}

	bids := make([]BookLevel, 0)
	for _, l := range bidMap { bids = append(bids, *l) }
	sort.Slice(bids, func(i, j int) bool { return bids[i].Price > bids[j].Price })

	asks := make([]BookLevel, 0)
	for _, l := range askMap { asks = append(asks, *l) }
	sort.Slice(asks, func(i, j int) bool { return asks[i].Price < asks[j].Price })

	var lastPrice *float64
	if lp, ok := ex.LastPrice[productID]; ok { lastPrice = &lp }

	return BookSnapshot{Bids: bids, Asks: asks, LastTradePrice: lastPrice}
}

func (ex *Exchange) getMidPrice(productID string) *float64 {
	var bestBid, bestAsk *float64
	for _, o := range ex.Orders {
		if o.ProductID != productID || o.Remaining() <= 0 { continue }
		if o.Side == "buy" && (bestBid == nil || o.Price > *bestBid) {
			p := o.Price; bestBid = &p
		}
		if o.Side == "sell" && (bestAsk == nil || o.Price < *bestAsk) {
			p := o.Price; bestAsk = &p
		}
	}
	if bestBid != nil && bestAsk != nil {
		mid := (*bestBid + *bestAsk) / 2
		return &mid
	}
	if lp, ok := ex.LastPrice[productID]; ok { return &lp }
	return nil
}

func (ex *Exchange) GetBotState(userID string) BotState {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.getBotStateInternal(userID)
}

func (ex *Exchange) getBotStateInternal(userID string) BotState {
	shared := ex.computeSharedStateLocked()
	return ex.buildUserStateLocked(userID, false, &shared).BotState
}

func (ex *Exchange) botProducts(products []*Product) []BotProduct {
	botProducts := make([]BotProduct, 0, len(products))
	for _, p := range products {
		bp := BotProduct{
			ID: p.ID, Symbol: p.Symbol, Name: p.Name, IsEtf: p.IsEtf, Kind: p.kind(),
			PositionLimit: p.PositionLimit, TickSize: p.TickSize,
		}
		if p.IsEtf {
			for _, comp := range p.getCompositions() {
				var bc []BotComposition
				for _, c := range comp {
					cp := ex.Products[c.ProductID]
					sym := "???"
					if cp != nil { sym = cp.Symbol }
					bc = append(bc, BotComposition{Symbol: sym, Weight: c.Weight})
				}
				bp.Compositions = append(bp.Compositions, bc)
			}
		}
		if p.kind() == KindOption { bp.Option = ex.optionInfo(p) }
		botProducts = append(botProducts, bp)
	}
	return botProducts
}

var kindOrder = map[string]int{KindFruit: 0, KindEtf: 1, KindOption: 2}

func (ex *Exchange) computeMidFromBook(book BookSnapshot) *float64 {
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		mid := (book.Bids[0].Price + book.Asks[0].Price) / 2
		return &mid
	}
	return book.LastTradePrice
}

func (ex *Exchange) activeProducts() []*Product {
	var result []*Product
	for _, p := range ex.Products {
		if p.IsActive { result = append(result, p) }
	}
	sort.Slice(result, func(i, j int) bool {
		ki, kj := kindOrder[result[i].kind()], kindOrder[result[j].kind()]
		if ki != kj { return ki < kj }
		if result[i].ExpiryMin != result[j].ExpiryMin { return result[i].ExpiryMin < result[j].ExpiryMin }
		return result[i].Symbol < result[j].Symbol
	})
	return result
}

func (ex *Exchange) GetAllProducts() []map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	var result []map[string]interface{}
	for _, p := range ex.activeProducts() {
		m := map[string]interface{}{
			"_id": p.ID, "symbol": p.Symbol, "name": p.Name,
			"isEtf": p.IsEtf, "positionLimit": p.PositionLimit,
			"isActive": p.IsActive, "tickSize": p.TickSize, "kind": p.kind(),
		}
		if p.IsEtf {
			m["etfCompositions"] = p.getCompositions()
		}
		if p.kind() == KindOption {
			m["option"] = ex.optionInfo(p)
		}
		result = append(result, m)
	}
	return result
}

type VolumeMatrixEntry struct {
	Buyer  string  `json:"buyer"`
	Seller string  `json:"seller"`
	Volume float64 `json:"volume"`
	Qty    int     `json:"qty"`
}

func (ex *Exchange) GetVolumeMatrix() []VolumeMatrixEntry {
	ex.mu.RLock()
	defer ex.mu.RUnlock()

	type pairKey struct{ buyer, seller string }
	agg := make(map[pairKey]*VolumeMatrixEntry)

	for _, f := range ex.Fills {
		buyUser := ex.Users[f.BuyUserID]
		sellUser := ex.Users[f.SellUserID]
		if buyUser == nil || sellUser == nil { continue }
		k := pairKey{buyUser.Username, sellUser.Username}
		if e, ok := agg[k]; ok {
			e.Volume += f.Price * float64(f.Qty)
			e.Qty += f.Qty
		} else {
			agg[k] = &VolumeMatrixEntry{
				Buyer:  buyUser.Username,
				Seller: sellUser.Username,
				Volume: f.Price * float64(f.Qty),
				Qty:    f.Qty,
			}
		}
	}

	result := make([]VolumeMatrixEntry, 0, len(agg))
	for _, e := range agg {
		result = append(result, *e)
	}
	return result
}

func (ex *Exchange) GetExchangeState() map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return map[string]interface{}{"isOpen": ex.IsOpen}
}

func (ex *Exchange) GetRecentTrades(userID string) []TradeInfo {
	ex.mu.RLock()
	defer ex.mu.RUnlock()

	var result []TradeInfo
	start := len(ex.Fills) - 100
	if start < 0 { start = 0 }
	for i := len(ex.Fills) - 1; i >= start; i-- {
		f := ex.Fills[i]
		p := ex.Products[f.ProductID]
		sym := "???"
		if p != nil { sym = p.Symbol }
		buyer := "?"
		if u := ex.Users[f.BuyUserID]; u != nil { buyer = u.Username }
		seller := "?"
		if u := ex.Users[f.SellUserID]; u != nil { seller = u.Username }
		isMine := userID != "" && (f.BuyUserID == userID || f.SellUserID == userID)
		var mySide *string
		if userID != "" {
			if f.BuyUserID == userID { s := "buy"; mySide = &s }
			if f.SellUserID == userID { s := "sell"; mySide = &s }
		}
		result = append(result, TradeInfo{
			ID: f.ID, Symbol: sym, Price: f.Price, Qty: f.Qty,
			CreatedAt: f.CreatedAt, IsMine: isMine, MySide: mySide,
			Buyer: buyer, Seller: seller,
		})
	}
	return result
}

// --- Leaderboard ---

func (ex *Exchange) GetLeaderboard() []LeaderboardEntry {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.leaderboard(ex.markPrices())
}

func (ex *Exchange) leaderboard(marks map[string]float64) []LeaderboardEntry {
	stats := ex.scoreStats()
	result := make([]LeaderboardEntry, 0)
	anyDays := false
	for _, u := range ex.Users {
		if u.IsHouse { continue }
		e := LeaderboardEntry{UserID: u.ID, Username: u.Username}
		fees := ex.account(u.ID).Fees
		for pid := range ex.Products {
			if pos := ex.Positions[posKey(u.ID, pid)]; pos != nil { e.RealizedPnl += pos.RealizedPnl }
		}
		e.RealizedPnl -= fees
		e.TotalPnl = ex.userPnl(u.ID, marks)
		e.UnrealizedPnl = e.TotalPnl - e.RealizedPnl
		e.Busts = ex.account(u.ID).Busts
		if st := stats[u.ID]; st != nil {
			e.Days, e.MeanDailyPnl, e.StdDailyPnl, e.Score = st.Days, st.Mean, st.Std, st.Score
			e.Busts += st.Busts
			anyDays = true
		}
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if anyDays && (a.Days > 0) != (b.Days > 0) { return a.Days > 0 }
		if anyDays && a.Score != b.Score { return a.Score > b.Score }
		if a.TotalPnl != b.TotalPnl { return a.TotalPnl > b.TotalPnl }
		return a.Username < b.Username
	})
	return result
}

func (ex *Exchange) allPositions(marks map[string]float64) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, pos := range ex.Positions {
		if pos.Qty == 0 && pos.RealizedPnl == 0 { continue }
		u := ex.Users[pos.UserID]
		p := ex.Products[pos.ProductID]
		if u == nil || p == nil || u.IsHouse { continue }
		unrealized := 0.0
		if pos.Qty != 0 { unrealized = (markFor(pos, marks) - pos.AvgEntryPrice) * float64(pos.Qty) }
		result = append(result, map[string]interface{}{
			"username": u.Username, "symbol": p.Symbol,
			"qty": pos.Qty, "avgEntryPrice": pos.AvgEntryPrice,
			"realizedPnl": pos.RealizedPnl, "unrealizedPnl": unrealized,
		})
	}
	return result
}

func (ex *Exchange) userList() []map[string]interface{} {
	users := make([]map[string]interface{}, 0)
	for _, u := range ex.Users {
		if u.IsHouse { continue }
		users = append(users, map[string]interface{}{
			"_id": u.ID, "username": u.Username, "isAdmin": u.IsAdmin,
			"noPositionLimit": u.NoPositionLimit, "rateLimitMs": u.RateLimitMs,
		})
	}
	return users
}

func (ex *Exchange) GetPnlHistory() map[string][]map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()

	// Group by username
	byUser := make(map[string][]PnlSnapshot)
	for _, s := range ex.PnlSnaps {
		u := ex.Users[s.UserID]
		if u == nil { continue }
		byUser[u.Username] = append(byUser[u.Username], s)
	}

	const maxPerUser = 500
	result := make(map[string][]map[string]interface{})
	for uname, snaps := range byUser {
		if len(snaps) <= maxPerUser {
			out := make([]map[string]interface{}, len(snaps))
			for i, s := range snaps {
				out[i] = map[string]interface{}{"snapshotAt": s.SnapshotAt, "totalPnl": s.TotalPnl, "realizedPnl": s.RealizedPnl}
			}
			result[uname] = out
		} else {
			out := make([]map[string]interface{}, maxPerUser)
			for i := 0; i < maxPerUser; i++ {
				idx := i * (len(snaps) - 1) / (maxPerUser - 1)
				s := snaps[idx]
				out[i] = map[string]interface{}{"snapshotAt": s.SnapshotAt, "totalPnl": s.TotalPnl, "realizedPnl": s.RealizedPnl}
			}
			result[uname] = out
		}
	}
	return result
}

func (ex *Exchange) GetAllPositions() []map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.allPositions(ex.markPrices())
}

func (ex *Exchange) SnapshotPnl() {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	now := time.Now().UnixMilli()
	marks := ex.markPrices()
	for _, u := range ex.Users {
		if u.IsHouse { continue }
		realized := -ex.account(u.ID).Fees
		for pid := range ex.Products {
			if pos := ex.Positions[posKey(u.ID, pid)]; pos != nil { realized += pos.RealizedPnl }
		}
		ex.PnlSnaps = append(ex.PnlSnaps, PnlSnapshot{
			UserID: u.ID, TotalPnl: ex.userPnl(u.ID, marks), RealizedPnl: realized, SnapshotAt: now,
		})
	}

	const maxPnlSnaps = 50000
	if len(ex.PnlSnaps) > maxPnlSnaps {
		cutoff := now - 5*60*1000
		var recent, old []PnlSnapshot
		for _, s := range ex.PnlSnaps {
			if s.SnapshotAt >= cutoff {
				recent = append(recent, s)
			} else {
				old = append(old, s)
			}
		}
		type bucketKey struct {
			UserID string
			Bucket int64
		}
		seen := make(map[bucketKey]bool)
		var kept []PnlSnapshot
		for _, s := range old {
			k := bucketKey{s.UserID, s.SnapshotAt / 10000}
			if !seen[k] {
				seen[k] = true
				kept = append(kept, s)
			}
		}
		ex.PnlSnaps = append(kept, recent...)
	}
}

func (ex *Exchange) SnapshotPrices() {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	now := time.Now().UnixMilli()
	for _, p := range ex.Products {
		mid := ex.getMidPrice(p.ID)
		if mid == nil { continue }
		ex.PriceSnaps = append(ex.PriceSnaps, PriceSnapshot{
			Symbol: p.Symbol, Mid: *mid, SnapshotAt: now,
		})
	}

	// Downsample when slice gets large: keep last 5 min at full res,
	// older data at 1 point per 10s per symbol.
	const maxSnaps = 50000
	if len(ex.PriceSnaps) > maxSnaps {
		cutoff := now - 5*60*1000
		var recent, old []PriceSnapshot
		for _, s := range ex.PriceSnaps {
			if s.SnapshotAt >= cutoff {
				recent = append(recent, s)
			} else {
				old = append(old, s)
			}
		}
		// Downsample old: keep one per symbol per 10s bucket
		type bucketKey struct {
			Symbol string
			Bucket int64
		}
		seen := make(map[bucketKey]bool)
		var kept []PriceSnapshot
		for _, s := range old {
			k := bucketKey{s.Symbol, s.SnapshotAt / 10000}
			if !seen[k] {
				seen[k] = true
				kept = append(kept, s)
			}
		}
		ex.PriceSnaps = append(kept, recent...)
	}
}

func (ex *Exchange) GetPriceHistory() map[string][]map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()

	// Group by symbol
	bySymbol := make(map[string][]PriceSnapshot)
	for _, s := range ex.PriceSnaps {
		bySymbol[s.Symbol] = append(bySymbol[s.Symbol], s)
	}

	// Return at most 500 points per symbol (evenly sampled)
	const maxPerSymbol = 500
	result := make(map[string][]map[string]interface{})
	for sym, snaps := range bySymbol {
		if len(snaps) <= maxPerSymbol {
			out := make([]map[string]interface{}, len(snaps))
			for i, s := range snaps {
				out[i] = map[string]interface{}{"mid": s.Mid, "snapshotAt": s.SnapshotAt}
			}
			result[sym] = out
		} else {
			out := make([]map[string]interface{}, maxPerSymbol)
			for i := 0; i < maxPerSymbol; i++ {
				idx := i * (len(snaps) - 1) / (maxPerSymbol - 1)
				s := snaps[idx]
				out[i] = map[string]interface{}{"mid": s.Mid, "snapshotAt": s.SnapshotAt}
			}
			result[sym] = out
		}
	}
	return result
}

// --- Full State for WebSocket (includes leaderboard etc) ---

type FullWsState struct {
	BotState
	RecentTrades []TradeInfo                      `json:"recentTrades"`
	Leaderboard  []LeaderboardEntry               `json:"leaderboard"`
	AllPositions []map[string]interface{}          `json:"allPositions"`
	AllUsers     []map[string]interface{}          `json:"allUsers,omitempty"`
}

type SharedState struct {
	Products     []BotProduct
	Books        map[string]BookSnapshot
	ExchangeOpen bool
	Trades       []TradeInfo
	Leaderboard  []LeaderboardEntry
	AllPositions []map[string]interface{}
	AllUsers     []map[string]interface{}
	Windows      []OptionWindow
	marks        map[string]float64
}

func (ex *Exchange) ComputeSharedState() SharedState {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.computeSharedStateLocked()
}

func (ex *Exchange) computeSharedStateLocked() SharedState {
	products := ex.activeProducts()
	books := make(map[string]BookSnapshot)
	for _, p := range products {
		books[p.Symbol] = ex.getBookSnapshot(p.ID)
	}

	trades := make([]TradeInfo, 0)
	start := len(ex.Fills) - 100
	if start < 0 { start = 0 }
	for i := len(ex.Fills) - 1; i >= start; i-- {
		f := ex.Fills[i]
		p := ex.Products[f.ProductID]
		sym := "???"; if p != nil { sym = p.Symbol }
		buyer := "?"; if u := ex.Users[f.BuyUserID]; u != nil { buyer = u.Username }
		seller := "?"; if u := ex.Users[f.SellUserID]; u != nil { seller = u.Username }
		trades = append(trades, TradeInfo{
			ID: f.ID, Symbol: sym, Price: f.Price, Qty: f.Qty,
			CreatedAt: f.CreatedAt, Buyer: buyer, Seller: seller,
		})
	}

	marks := ex.markPrices()
	return SharedState{
		Products: ex.botProducts(products), Books: books, ExchangeOpen: ex.IsOpen,
		Trades: trades, Leaderboard: ex.leaderboard(marks), AllPositions: ex.allPositions(marks),
		AllUsers: ex.userList(), Windows: ex.recentWindows(0, 30), marks: marks,
	}
}

func (ex *Exchange) BuildUserState(userID string, isAdmin bool, shared *SharedState) *FullWsState {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.buildUserStateLocked(userID, isAdmin, shared)
}

func (ex *Exchange) buildUserStateLocked(userID string, isAdmin bool, shared *SharedState) *FullWsState {
	u := ex.Users[userID]
	if u == nil { return nil }
	marks := shared.marks
	if marks == nil { marks = ex.markPrices() }

	positions := make([]PositionInfo, 0)
	for _, bp := range shared.Products {
		pos := ex.Positions[posKey(userID, bp.ID)]
		if pos == nil { continue }
		unrealized := 0.0
		if pos.Qty != 0 { unrealized = (markFor(pos, marks) - pos.AvgEntryPrice) * float64(pos.Qty) }
		positions = append(positions, PositionInfo{
			ProductID: bp.ID, Symbol: bp.Symbol, Qty: pos.Qty,
			RealizedPnl: pos.RealizedPnl, AvgEntryPrice: pos.AvgEntryPrice,
			UnrealizedPnl: unrealized,
		})
	}

	openOrders := make([]UserOrderInfo, 0)
	for _, o := range ex.Orders {
		if o.UserID != userID { continue }
		p := ex.Products[o.ProductID]
		sym := "???"; if p != nil { sym = p.Symbol }
		status := "open"
		if o.FilledQty > 0 { status = "partial" }
		openOrders = append(openOrders, UserOrderInfo{
			ID: o.ID, ProductID: o.ProductID, Symbol: sym,
			Side: o.Side, Price: o.Price, Qty: o.Qty,
			FilledQty: o.FilledQty, Status: status,
			OrderType: o.OrderType, CreatedAt: o.CreatedAt,
		})
	}

	trades := make([]TradeInfo, len(shared.Trades))
	for i, t := range shared.Trades {
		trades[i] = t
		if t.Buyer == u.Username {
			trades[i].IsMine = true
			s := "buy"; trades[i].MySide = &s
		} else if t.Seller == u.Username {
			trades[i].IsMine = true
			s := "sell"; trades[i].MySide = &s
		}
	}

	acct := ex.accountInfo(userID, marks)
	ws := FullWsState{
		BotState: BotState{
			ExchangeOpen:  shared.ExchangeOpen,
			ServerTime:    time.Now().UnixMilli(),
			Products:      shared.Products,
			Books:         shared.Books,
			Positions:     positions,
			OpenOrders:    openOrders,
			Pnl:           acct.Pnl,
			Account:       acct,
			Config:        ex.publicConfig(isAdmin),
			OptionWindows: shared.Windows,
		},
		RecentTrades: trades,
		Leaderboard:  shared.Leaderboard,
		AllPositions: shared.AllPositions,
	}
	if isAdmin {
		ws.AllUsers = shared.AllUsers
	}
	return &ws
}

func (ex *Exchange) GetFullState(userID string, isAdmin bool) *FullWsState {
	shared := ex.ComputeSharedState()
	return ex.BuildUserState(userID, isAdmin, &shared)
}

// --- Persistence ---

type SaveState struct {
	Users     map[string]*User     `json:"users"`
	Usernames map[string]string    `json:"usernames"`
	Sessions  map[string]string    `json:"sessions"`
	Products  map[string]*Product  `json:"products"`
	Symbols   map[string]string    `json:"symbols"`
	Orders    map[string]*Order    `json:"orders"`
	Positions map[string]*Position `json:"positions"`
	Fills     []*Fill              `json:"fills"`
	PnlSnaps   []PnlSnapshot        `json:"pnlSnaps"`
	PriceSnaps []PriceSnapshot      `json:"priceSnaps"`
	IsOpen     bool                 `json:"isOpen"`
	NextID     int64                `json:"nextId"`

	Config        *Config              `json:"config"`
	Accounts      map[string]*Account  `json:"accounts"`
	DailyResults  []DailyResult        `json:"dailyResults"`
	Options       map[int]*OptionState `json:"options"`
	OptionHistory []OptionWindow       `json:"optionHistory"`
	LastPrice     map[string]float64   `json:"lastPrice"`
	EtfTradeLog   []pricePoint         `json:"etfTradeLog"`
	EtfMidLog     []pricePoint         `json:"etfMidLog"`
	DayStart      int64                `json:"dayStart"`
	Sim           *SimState            `json:"sim"`
}

func (ex *Exchange) Save() error {
	ex.mu.RLock()
	data, err := json.Marshal(SaveState{
		Users: ex.Users, Usernames: ex.Usernames, Sessions: ex.Sessions,
		Products: ex.Products, Symbols: ex.Symbols, Orders: ex.Orders,
		Positions: ex.Positions, Fills: ex.Fills, PnlSnaps: ex.PnlSnaps, PriceSnaps: ex.PriceSnaps,
		IsOpen: ex.IsOpen, NextID: ex.nextID.Load(),
		Config: ex.Config, Accounts: ex.Accounts, DailyResults: ex.DailyResults,
		Options: ex.Options, OptionHistory: ex.OptionHistory, LastPrice: ex.LastPrice,
		EtfTradeLog: ex.EtfTradeLog, EtfMidLog: ex.EtfMidLog, DayStart: ex.DayStart, Sim: ex.Sim,
	})
	ex.mu.RUnlock()
	if err != nil { return err }
	tmp := ex.savePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil { return err }
	return os.Rename(tmp, ex.savePath)
}

func (ex *Exchange) Load() error {
	data, err := os.ReadFile(ex.savePath)
	if err != nil {
		if os.IsNotExist(err) { return nil }
		return err
	}
	var state SaveState
	if err := json.Unmarshal(data, &state); err != nil { return err }

	ex.mu.Lock()
	defer ex.mu.Unlock()
	ex.Users = state.Users
	ex.Usernames = state.Usernames
	ex.Sessions = state.Sessions
	ex.Products = state.Products
	ex.Symbols = state.Symbols
	ex.Orders = state.Orders
	ex.Positions = state.Positions
	ex.Fills = state.Fills
	ex.PnlSnaps = state.PnlSnaps
	ex.PriceSnaps = state.PriceSnaps
	ex.IsOpen = state.IsOpen
	ex.nextID.Store(state.NextID)
	ex.Config = state.Config
	ex.Accounts = state.Accounts
	ex.DailyResults = state.DailyResults
	ex.Options = state.Options
	ex.OptionHistory = state.OptionHistory
	ex.LastPrice = state.LastPrice
	ex.EtfTradeLog = state.EtfTradeLog
	ex.EtfMidLog = state.EtfMidLog
	ex.DayStart = state.DayStart
	ex.Sim = state.Sim

	if ex.Users == nil { ex.Users = make(map[string]*User) }
	if ex.Sessions == nil { ex.Sessions = make(map[string]string) }
	if ex.Usernames == nil { ex.Usernames = make(map[string]string) }
	if ex.Products == nil { ex.Products = make(map[string]*Product) }
	if ex.Symbols == nil { ex.Symbols = make(map[string]string) }
	if ex.Orders == nil { ex.Orders = make(map[string]*Order) }
	if ex.Positions == nil { ex.Positions = make(map[string]*Position) }

	for _, p := range ex.Products {
		if p.IsEtf && len(p.EtfCompositions) == 0 && len(p.EtfComposition) > 0 {
			p.EtfCompositions = [][]EtfComponent{p.EtfComposition}
			p.EtfComposition = nil
		}
	}
	return nil
}

func (ex *Exchange) saveAsync() {
	ex.dirty.Store(true)
}

func (ex *Exchange) SaveIfDirty() {
	if ex.dirty.CompareAndSwap(true, false) {
		ex.Save()
	}
}

func (ex *Exchange) doBroadcast() {
	if ex.broadcast != nil {
		if ex.broadcastPending.CompareAndSwap(false, true) {
			go func() {
				time.Sleep(100 * time.Millisecond)
				ex.broadcastPending.Store(false)
				ex.broadcast()
			}()
		}
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
