package main

// QFin 2026 sem 2 project rules: products, accounts, margin, trading days,
// up-down options and scoring. See PROJECT.md for the public contract.

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	KindFruit  = "fruit"
	KindEtf    = "etf"
	KindOption = "option"

	SourceLastTrade = "etf_last_trade"
	SourceMid       = "etf_mid"

	EtfSymbol = "etf"
)

var fruitSymbols = []string{"apples", "bananas", "oranges", "strawberries", "watermelon"}

var fruitNames = map[string]string{
	"apples": "Apples", "bananas": "Bananas", "oranges": "Oranges",
	"strawberries": "Strawberries", "watermelon": "Watermelon",
}

var allExpiries = []int{5, 15, 60}

var houseUsernames = []string{"house-mm", "house-flow", "house-informed", "house-options"}

// --- Types ---

type Config struct {
	Phase           string  `json:"phase"`
	StartingCash    float64 `json:"startingCash"`
	DayLengthMin    int     `json:"dayLengthMin"`
	EtfFee          float64 `json:"etfFee"`
	FruitMarginRate float64 `json:"fruitMarginRate"`
	EtfMarginRate   float64 `json:"etfMarginRate"`
	SimEnabled      bool    `json:"simEnabled"`
	EnabledExpiries []int   `json:"enabledExpiries"`
	// Stored inverted so configs saved before this field existed keep click trading on.
	ClickTradingOff bool `json:"clickTradingOff"`
}

func defaultConfig() *Config {
	return &Config{
		Phase:           "training",
		StartingCash:    10000,
		DayLengthMin:    1440,
		EtfFee:          0.05,
		FruitMarginRate: 0.2,
		EtfMarginRate:   0.2,
		SimEnabled:      true,
		EnabledExpiries: []int{5, 60},
	}
}

func (c *Config) expiryEnabled(e int) bool {
	for _, x := range c.EnabledExpiries {
		if x == e { return true }
	}
	return false
}

type PublicConfig struct {
	Phase           string             `json:"phase"`
	StartingCash    float64            `json:"startingCash"`
	DayLengthMin    int                `json:"dayLengthMin"`
	EtfFee          float64            `json:"etfFee"`
	MarginRates     map[string]float64 `json:"marginRates"`
	EnabledExpiries []int              `json:"enabledExpiries"`
	ClickTrading    bool               `json:"clickTrading"`
	SimEnabled      *bool              `json:"simEnabled,omitempty"`
}

func (ex *Exchange) publicConfig(isAdmin bool) PublicConfig {
	c := ex.Config
	pc := PublicConfig{
		Phase: c.Phase, StartingCash: c.StartingCash, DayLengthMin: c.DayLengthMin,
		EtfFee: c.EtfFee,
		MarginRates:     map[string]float64{KindFruit: c.FruitMarginRate, KindEtf: c.EtfMarginRate},
		EnabledExpiries: append([]int{}, c.EnabledExpiries...),
		ClickTrading:    !c.ClickTradingOff,
	}
	if isAdmin {
		v := c.SimEnabled
		pc.SimEnabled = &v
	}
	return pc
}

type Account struct {
	Cash      float64 `json:"cash"`
	StartCash float64 `json:"startCash"`
	Fees      float64 `json:"fees"`
	Busts     int     `json:"busts"`
	BustLoss  float64 `json:"bustLoss"`
	Active    bool    `json:"active"`
}

type AccountInfo struct {
	Cash            float64 `json:"cash"`
	Equity          float64 `json:"equity"`
	Pnl             float64 `json:"pnl"`
	StartingCash    float64 `json:"startingCash"`
	MarginUsed      float64 `json:"marginUsed"`
	MarginAvailable float64 `json:"marginAvailable"`
	Busts           int     `json:"busts"`
	Day             string  `json:"day"`
	DayStart        int64   `json:"dayStart"`
	DayEnd          int64   `json:"dayEnd"`
}

type DailyResult struct {
	UserID  string  `json:"userId"`
	Day     string  `json:"day"`
	Pnl     float64 `json:"pnl"`
	Busts   int     `json:"busts"`
	EndedAt int64   `json:"endedAt"`
}

type OptionState struct {
	ExpiryMin   int      `json:"expiryMin"`
	ProductID   string   `json:"productId"`
	WindowOpen  int64    `json:"windowOpen"`
	Strike      *float64 `json:"strike"`
	PriceSource string   `json:"priceSource"`
}

func (o *OptionState) seconds() int64 { return int64(o.ExpiryMin) * 60 }
func (o *OptionState) close() int64   { return o.WindowOpen + o.seconds() }

type OptionInfo struct {
	ExpiryMin   int      `json:"expiryMin"`
	WindowOpen  int64    `json:"windowOpen"`
	WindowClose int64    `json:"windowClose"`
	Strike      *float64 `json:"strike"`
	PriceSource string   `json:"priceSource"`
}

type OptionWindow struct {
	ExpiryMin   int     `json:"expiryMin"`
	WindowOpen  int64   `json:"windowOpen"`
	Strike      float64 `json:"strike"`
	Settle      float64 `json:"settle"`
	UpWon       bool    `json:"upWon"`
	PriceSource string  `json:"priceSource"`
}

type pricePoint struct {
	Sec   int64   `json:"s"`
	Price float64 `json:"p"`
}

// --- Setup ---

func (p *Product) kind() string {
	if p.Kind != "" { return p.Kind }
	if p.IsEtf { return KindEtf }
	return KindFruit
}

func roundTo(x, tick float64) float64 {
	return math.Round(x/tick) * tick
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// ensureProject creates everything the project needs. Idempotent. Caller holds ex.mu.
func (ex *Exchange) ensureProject() {
	if ex.Config == nil { ex.Config = defaultConfig() }
	if ex.Accounts == nil { ex.Accounts = make(map[string]*Account) }
	if ex.LastPrice == nil { ex.LastPrice = make(map[string]float64) }
	if ex.Options == nil { ex.Options = make(map[int]*OptionState) }

	for _, name := range houseUsernames {
		if _, ok := ex.Usernames[name]; ok {
			ex.Users[ex.Usernames[name]].IsHouse = true
			continue
		}
		u := &User{
			ID: ex.genID("u"), Username: name, Salt: randomHex(16),
			IsHouse: true, NoPositionLimit: true,
		}
		u.PasswordHash = hashPassword(randomHex(16), u.Salt)
		ex.Users[u.ID] = u
		ex.Usernames[name] = u.ID
	}

	var etfComp []EtfComponent
	for _, sym := range fruitSymbols {
		pid, ok := ex.Symbols[sym]
		if !ok {
			p := &Product{
				ID: ex.genID("p"), Symbol: sym, Name: fruitNames[sym], Kind: KindFruit,
				PositionLimit: 500, TickSize: 0.01, IsActive: true,
			}
			ex.Products[p.ID] = p
			ex.Symbols[sym] = p.ID
			pid = p.ID
		}
		etfComp = append(etfComp, EtfComponent{ProductID: pid, Weight: 1})
	}

	if _, ok := ex.Symbols[EtfSymbol]; !ok {
		p := &Product{
			ID: ex.genID("p"), Symbol: EtfSymbol, Name: "Fruit ETF", Kind: KindEtf,
			IsEtf: true, EtfCompositions: [][]EtfComponent{etfComp},
			PositionLimit: 200, TickSize: 0.01, IsActive: true,
		}
		ex.Products[p.ID] = p
		ex.Symbols[EtfSymbol] = p.ID
	}

	now := time.Now().Unix()
	for _, e := range allExpiries {
		sym := fmt.Sprintf("up%d", e)
		pid, ok := ex.Symbols[sym]
		if !ok {
			// Option products stay registered even when disabled so symbols are stable.
			for id, p := range ex.Products {
				if p.Symbol == sym && p.Kind == KindOption { pid, ok = id, true }
			}
		}
		if !ok {
			p := &Product{
				ID: ex.genID("p"), Symbol: sym, Name: fmt.Sprintf("ETF Up/Down %dm", e),
				Kind: KindOption, ExpiryMin: e, PositionLimit: 5000, TickSize: 0.01,
			}
			ex.Products[p.ID] = p
			pid = p.ID
		}
		p := ex.Products[pid]
		wasActive := p.IsActive
		p.IsActive = ex.Config.expiryEnabled(e)
		if p.IsActive {
			ex.Symbols[sym] = pid
		} else {
			delete(ex.Symbols, sym)
		}
		if _, ok := ex.Options[e]; !ok {
			src := SourceLastTrade
			if e == 15 { src = SourceMid }
			ex.Options[e] = &OptionState{ExpiryMin: e, ProductID: pid, PriceSource: src}
		}
		os := ex.Options[e]
		os.ProductID = pid
		if os.WindowOpen == 0 || (p.IsActive && !wasActive) {
			ex.openWindow(os, now)
		}
	}

	if ex.DayStart == 0 { ex.DayStart = now }
	for _, u := range ex.Users {
		if !u.IsHouse { ex.account(u.ID) }
	}
	ex.ensureSim()
}

func (ex *Exchange) etfProduct() *Product {
	if pid, ok := ex.Symbols[EtfSymbol]; ok { return ex.Products[pid] }
	return nil
}

func (ex *Exchange) isHouse(userID string) bool {
	u := ex.Users[userID]
	return u != nil && u.IsHouse
}

func (ex *Exchange) houseID(name string) string { return ex.Usernames[name] }

// --- Accounts ---

func (ex *Exchange) account(userID string) *Account {
	a, ok := ex.Accounts[userID]
	if !ok {
		start := ex.Config.StartingCash
		a = &Account{Cash: start, StartCash: start}
		ex.Accounts[userID] = a
	}
	return a
}

// markPrices returns book mid, else last trade, for every product.
func (ex *Exchange) markPrices() map[string]float64 {
	bestBid := make(map[string]float64)
	bestAsk := make(map[string]float64)
	for _, o := range ex.Orders {
		if o.Remaining() <= 0 { continue }
		if o.Side == "buy" {
			if b, ok := bestBid[o.ProductID]; !ok || o.Price > b { bestBid[o.ProductID] = o.Price }
		} else {
			if a, ok := bestAsk[o.ProductID]; !ok || o.Price < a { bestAsk[o.ProductID] = o.Price }
		}
	}
	marks := make(map[string]float64, len(ex.Products))
	for id := range ex.Products {
		b, hb := bestBid[id]
		a, ha := bestAsk[id]
		if hb && ha {
			marks[id] = (a + b) / 2
		} else if lp, ok := ex.LastPrice[id]; ok {
			marks[id] = lp
		}
	}
	return marks
}

func markFor(pos *Position, marks map[string]float64) float64 {
	if m, ok := marks[pos.ProductID]; ok { return m }
	return pos.AvgEntryPrice
}

func (ex *Exchange) equity(userID string, marks map[string]float64) float64 {
	eq := ex.account(userID).Cash
	for pid := range ex.Products {
		pos := ex.Positions[posKey(userID, pid)]
		if pos == nil || pos.Qty == 0 { continue }
		eq += float64(pos.Qty) * markFor(pos, marks)
	}
	return eq
}

// userPnl is today's pnl including losses from earlier busts.
func (ex *Exchange) userPnl(userID string, marks map[string]float64) float64 {
	a := ex.account(userID)
	return ex.equity(userID, marks) - a.StartCash + a.BustLoss
}

type hypoOrder struct {
	productID string
	side      string
	price     float64
	qty       int
}

// marginRequired is the margin for the worst case of position + resting orders (+ extra).
func (ex *Exchange) marginRequired(userID string, marks map[string]float64, extra *hypoOrder) float64 {
	restBuy := make(map[string]int)
	restSell := make(map[string]int)
	for _, o := range ex.Orders {
		if o.UserID != userID { continue }
		if o.Side == "buy" { restBuy[o.ProductID] += o.Remaining() } else { restSell[o.ProductID] += o.Remaining() }
	}
	if extra != nil {
		if extra.side == "buy" { restBuy[extra.productID] += extra.qty } else { restSell[extra.productID] += extra.qty }
	}

	total := 0.0
	for pid, p := range ex.Products {
		q := 0
		avg := 0.0
		if pos := ex.Positions[posKey(userID, pid)]; pos != nil { q, avg = pos.Qty, pos.AvgEntryPrice }
		long := q + restBuy[pid]
		short := q - restSell[pid]
		if long == 0 && short == 0 { continue }

		m, ok := marks[pid]
		if !ok {
			if extra != nil && extra.productID == pid {
				m = extra.price
			} else {
				m = avg
			}
		}

		if p.kind() == KindOption {
			total += math.Max(optionLoss(long, m), optionLoss(short, m))
			continue
		}
		rate := ex.Config.FruitMarginRate
		if p.kind() == KindEtf { rate = ex.Config.EtfMarginRate }
		worst := math.Max(math.Abs(float64(long)), math.Abs(float64(short)))
		total += rate * m * worst
	}
	return total
}

func optionLoss(q int, mark float64) float64 {
	if q > 0 { return float64(q) * mark }
	return float64(-q) * (1 - mark)
}

func (ex *Exchange) accountInfo(userID string, marks map[string]float64) AccountInfo {
	a := ex.account(userID)
	eq := ex.equity(userID, marks)
	used := ex.marginRequired(userID, marks, nil)
	start, end := ex.dayBounds()
	return AccountInfo{
		Cash: a.Cash, Equity: eq, Pnl: eq - a.StartCash + a.BustLoss,
		StartingCash: a.StartCash, MarginUsed: used, MarginAvailable: eq - used,
		Busts: a.Busts, Day: ex.dayLabel(), DayStart: start * 1000, DayEnd: end * 1000,
	}
}

// resetUserDay flattens a user and gives them a fresh starting balance. Caller holds ex.mu.
func (ex *Exchange) resetUserDay(userID string) {
	for id, o := range ex.Orders {
		if o.UserID == userID { delete(ex.Orders, id) }
	}
	for k, p := range ex.Positions {
		if p.UserID == userID { delete(ex.Positions, k) }
	}
	start := ex.Config.StartingCash
	ex.Accounts[userID] = &Account{Cash: start, StartCash: start}
}

func (ex *Exchange) checkBusts(marks map[string]float64) {
	for uid, u := range ex.Users {
		if u.IsHouse { continue }
		a := ex.account(uid)
		eq := ex.equity(uid, marks)
		if eq > 0 { continue }
		busts := a.Busts + 1
		loss := a.BustLoss + (eq - a.StartCash)
		ex.resetUserDay(uid)
		na := ex.Accounts[uid]
		na.Busts, na.BustLoss, na.Active = busts, loss, true
	}
}

// --- Trading days ---

func (ex *Exchange) dayLenSec() int64 {
	d := int64(ex.Config.DayLengthMin) * 60
	if d <= 0 { d = 86400 }
	return d
}

func (ex *Exchange) dayBounds() (int64, int64) {
	d := ex.dayLenSec()
	return ex.DayStart, (ex.DayStart/d + 1) * d
}

func (ex *Exchange) dayLabel() string {
	t := time.Unix(ex.DayStart, 0).UTC()
	// A day cut short by "End day now" starts mid-period; include the time so labels stay unique.
	if ex.dayLenSec() == 86400 && ex.DayStart%86400 == 0 { return t.Format("2006-01-02") }
	return t.Format("2006-01-02 15:04")
}

// endDay records everyone's pnl and resets them. Caller holds ex.mu.
func (ex *Exchange) endDay(now int64) {
	marks := ex.markPrices()
	day := ex.dayLabel()
	for uid, u := range ex.Users {
		if u.IsHouse { continue }
		a := ex.account(uid)
		pnl := ex.userPnl(uid, marks)
		hasPos := false
		for pid := range ex.Products {
			if pos := ex.Positions[posKey(uid, pid)]; pos != nil && (pos.Qty != 0 || pos.RealizedPnl != 0) { hasPos = true }
		}
		if a.Active || hasPos || math.Abs(pnl) > 1e-9 {
			ex.DailyResults = append(ex.DailyResults, DailyResult{
				UserID: uid, Day: day, Pnl: pnl, Busts: a.Busts, EndedAt: now * 1000,
			})
		}
		ex.resetUserDay(uid)
	}
	ex.DayStart = now
	ex.saveAsync()
	ex.doBroadcast()
}

func (ex *Exchange) EndDayNow(token string) error {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return err }
	if !u.IsAdmin { return fmt.Errorf("admin required") }
	ex.endDay(time.Now().Unix())
	return nil
}

type ScoreStats struct {
	Days  int
	Mean  float64
	Std   float64
	Score float64
	Busts int
}

func (ex *Exchange) scoreStats() map[string]*ScoreStats {
	byUser := make(map[string][]DailyResult)
	for _, r := range ex.DailyResults {
		byUser[r.UserID] = append(byUser[r.UserID], r)
	}
	out := make(map[string]*ScoreStats)
	for uid, rs := range byUser {
		s := &ScoreStats{Days: len(rs)}
		for _, r := range rs {
			s.Mean += r.Pnl
			s.Busts += r.Busts
		}
		s.Mean /= float64(len(rs))
		for _, r := range rs {
			s.Std += (r.Pnl - s.Mean) * (r.Pnl - s.Mean)
		}
		s.Std = math.Sqrt(s.Std / float64(len(rs)))
		s.Score = s.Mean - s.Std
		out[uid] = s
	}
	return out
}

func (ex *Exchange) GetDailyResults() map[string][]map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	out := make(map[string][]map[string]interface{})
	for _, r := range ex.DailyResults {
		u := ex.Users[r.UserID]
		if u == nil { continue }
		out[u.Username] = append(out[u.Username], map[string]interface{}{
			"day": r.Day, "pnl": r.Pnl, "busts": r.Busts, "endedAt": r.EndedAt,
		})
	}
	return out
}

// --- Config ---

func (ex *Exchange) GetConfig(isAdmin bool) PublicConfig {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.publicConfig(isAdmin)
}

func (ex *Exchange) UpdateConfig(token string, body map[string]interface{}) (PublicConfig, error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	u, err := ex.Authenticate(token)
	if err != nil { return PublicConfig{}, err }
	if !u.IsAdmin { return PublicConfig{}, fmt.Errorf("admin required") }

	c := *ex.Config
	num := func(key string, dst *float64, min float64) error {
		v, ok := body[key]
		if !ok { return nil }
		f, ok := v.(float64)
		if !ok || f < min { return fmt.Errorf("%s must be a number >= %v", key, min) }
		*dst = f
		return nil
	}
	if err := num("etfFee", &c.EtfFee, 0); err != nil { return PublicConfig{}, err }
	if err := num("fruitMarginRate", &c.FruitMarginRate, 0); err != nil { return PublicConfig{}, err }
	if err := num("etfMarginRate", &c.EtfMarginRate, 0); err != nil { return PublicConfig{}, err }
	if err := num("startingCash", &c.StartingCash, 1); err != nil { return PublicConfig{}, err }
	if v, ok := body["dayLengthMin"]; ok {
		f, ok := v.(float64)
		if !ok || f < 1 { return PublicConfig{}, fmt.Errorf("dayLengthMin must be >= 1") }
		c.DayLengthMin = int(f)
	}
	if v, ok := body["phase"]; ok {
		s, _ := v.(string)
		if s != "training" && s != "testing" { return PublicConfig{}, fmt.Errorf("phase must be training or testing") }
		c.Phase = s
	}
	if v, ok := body["simEnabled"]; ok {
		b, ok := v.(bool)
		if !ok { return PublicConfig{}, fmt.Errorf("simEnabled must be a bool") }
		c.SimEnabled = b
	}
	if v, ok := body["clickTrading"]; ok {
		b, ok := v.(bool)
		if !ok { return PublicConfig{}, fmt.Errorf("clickTrading must be a bool") }
		c.ClickTradingOff = !b
	}
	if v, ok := body["enabledExpiries"]; ok {
		arr, ok := v.([]interface{})
		if !ok { return PublicConfig{}, fmt.Errorf("enabledExpiries must be a list") }
		var es []int
		for _, x := range arr {
			f, _ := x.(float64)
			e := int(f)
			if e != 5 && e != 15 && e != 60 { return PublicConfig{}, fmt.Errorf("expiries must be 5, 15 or 60") }
			es = append(es, e)
		}
		sort.Ints(es)
		c.EnabledExpiries = es
	}

	ex.Config = &c
	// Re-sync option products with the enabled expiries; disabling one cancels its orders
	// and refunds open positions at entry price.
	for _, e := range allExpiries {
		os := ex.Options[e]
		p := ex.Products[os.ProductID]
		if p.IsActive && !c.expiryEnabled(e) {
			ex.voidWindow(os)
		}
	}
	ex.ensureProject()
	ex.saveAsync()
	ex.doBroadcast()
	return ex.publicConfig(true), nil
}

// --- Up-down options ---

func (ex *Exchange) optionInfo(p *Product) *OptionInfo {
	os := ex.Options[p.ExpiryMin]
	if os == nil { return nil }
	return &OptionInfo{
		ExpiryMin: os.ExpiryMin, WindowOpen: os.WindowOpen, WindowClose: os.close(),
		Strike: os.Strike, PriceSource: os.PriceSource,
	}
}

func (ex *Exchange) optionStateFor(productID string) *OptionState {
	for _, os := range ex.Options {
		if os.ProductID == productID { return os }
	}
	return nil
}

func (ex *Exchange) recordEtfTrade(sec int64, price float64) {
	ex.EtfTradeLog = append(ex.EtfTradeLog, pricePoint{sec, price})
	if len(ex.EtfTradeLog) > 20000 { ex.EtfTradeLog = ex.EtfTradeLog[len(ex.EtfTradeLog)-10000:] }
}

func (ex *Exchange) recordEtfMid(sec int64, mid float64) {
	ex.EtfMidLog = append(ex.EtfMidLog, pricePoint{sec, mid})
	if len(ex.EtfMidLog) > 8000 { ex.EtfMidLog = ex.EtfMidLog[len(ex.EtfMidLog)-4000:] }
}

// etfPriceAt returns the ETF price for a given source at or before second sec.
func (ex *Exchange) etfPriceAt(source string, sec int64) *float64 {
	log := ex.EtfTradeLog
	if source == SourceMid { log = ex.EtfMidLog }
	for i := len(log) - 1; i >= 0; i-- {
		if log[i].Sec <= sec {
			p := log[i].Price
			return &p
		}
	}
	return nil
}

func (ex *Exchange) openWindow(os *OptionState, now int64) {
	os.WindowOpen = (now / os.seconds()) * os.seconds()
	os.Strike = ex.etfPriceAt(os.PriceSource, os.WindowOpen)
	delete(ex.LastPrice, os.ProductID)
}

func (ex *Exchange) cancelProductOrders(productID string) {
	for id, o := range ex.Orders {
		if o.ProductID == productID { delete(ex.Orders, id) }
	}
}

// closeOptionPositions cash settles every position in an option product at price(pos).
func (ex *Exchange) closeOptionPositions(productID string, price func(*Position) float64) {
	for _, pos := range ex.Positions {
		if pos.ProductID != productID || pos.Qty == 0 { continue }
		px := price(pos)
		if pos.Qty > 0 {
			ex.updatePosition(pos.UserID, productID, pos.Qty, px, "sell")
		} else {
			ex.updatePosition(pos.UserID, productID, -pos.Qty, px, "buy")
		}
	}
}

func (ex *Exchange) voidWindow(os *OptionState) {
	ex.cancelProductOrders(os.ProductID)
	ex.closeOptionPositions(os.ProductID, func(p *Position) float64 { return p.AvgEntryPrice })
}

// rollOptions settles every window whose close second has fully ended. Caller holds ex.mu.
func (ex *Exchange) rollOptions(endedSec int64) {
	for _, e := range allExpiries {
		os := ex.Options[e]
		if os == nil || !ex.Config.expiryEnabled(e) { continue }
		if os.close() > endedSec { continue }

		settle := ex.etfPriceAt(os.PriceSource, os.close())
		if os.Strike == nil || settle == nil {
			ex.voidWindow(os)
		} else {
			upWon := *settle >= *os.Strike
			payout := 0.0
			if upWon { payout = 1 }
			ex.cancelProductOrders(os.ProductID)
			ex.closeOptionPositions(os.ProductID, func(*Position) float64 { return payout })
			w := OptionWindow{
				ExpiryMin: e, WindowOpen: os.WindowOpen, Strike: *os.Strike,
				Settle: *settle, UpWon: upWon, PriceSource: os.PriceSource,
			}
			ex.OptionHistory = append(ex.OptionHistory, w)
			if len(ex.OptionHistory) > 5000 { ex.OptionHistory = ex.OptionHistory[len(ex.OptionHistory)-4000:] }
			if ex.rec != nil { ex.rec.addWindow(w, os.close()) }
		}
		ex.openWindow(os, endedSec)
	}
}

func (ex *Exchange) recentWindows(expiry, limit int) []OptionWindow {
	out := make([]OptionWindow, 0)
	for i := len(ex.OptionHistory) - 1; i >= 0 && len(out) < limit; i-- {
		w := ex.OptionHistory[i]
		if expiry != 0 && w.ExpiryMin != expiry { continue }
		out = append(out, w)
	}
	return out
}

func (ex *Exchange) GetOptionWindows(expiry, limit int) []OptionWindow {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.recentWindows(expiry, limit)
}

// --- Per-second clock ---

// OnSecond runs once at the start of every wall-clock second, for the second that just ended.
func (ex *Exchange) OnSecond(endedSec int64) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	if etf := ex.etfProduct(); etf != nil {
		if mid := ex.getMidPrice(etf.ID); mid != nil { ex.recordEtfMid(endedSec, *mid) }
	}
	if ex.rec != nil { ex.recordTopOfBook(endedSec) }

	ex.rollOptions(endedSec)

	// Bootstrap only: a window that opened before any ETF price existed takes the first one.
	for _, os := range ex.Options {
		if os.Strike == nil && ex.Config.expiryEnabled(os.ExpiryMin) {
			os.Strike = ex.etfPriceAt(os.PriceSource, endedSec)
		}
	}

	if ex.IsOpen && ex.Config.SimEnabled { ex.simSecond(endedSec) }

	ex.checkBusts(ex.markPrices())

	if _, end := ex.dayBounds(); endedSec+1 >= end {
		ex.endDay(end)
	}
	ex.saveAsync()
}
