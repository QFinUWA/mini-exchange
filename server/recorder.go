package main

// Records market data every second and writes Parquet files with the same layout and
// column types as the handout data: <dir>/<table>/date=YYYY-MM-DD.parquet

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"
)

type TobRow struct {
	Ts      time.Time `parquet:"ts,timestamp(nanosecond)"`
	Product string    `parquet:"product"`
	Bid     *float64  `parquet:"bid,optional"`
	Ask     *float64  `parquet:"ask,optional"`
	BidSize int16     `parquet:"bid_size"`
	AskSize int16     `parquet:"ask_size"`
}

type TradeRow struct {
	Ts        time.Time `parquet:"ts,timestamp(nanosecond)"`
	Product   string    `parquet:"product"`
	Price     float64   `parquet:"price"`
	Size      int16     `parquet:"size"`
	Aggressor string    `parquet:"aggressor"`
}

type OptTobRow struct {
	Ts         time.Time `parquet:"ts,timestamp(nanosecond)"`
	ExpiryMin  int16     `parquet:"expiry_min"`
	WindowOpen time.Time `parquet:"window_open,timestamp(nanosecond)"`
	Bid        *float64  `parquet:"bid,optional"`
	Ask        *float64  `parquet:"ask,optional"`
	BidSize    int16     `parquet:"bid_size"`
	AskSize    int16     `parquet:"ask_size"`
}

type OptTradeRow struct {
	Ts         time.Time `parquet:"ts,timestamp(nanosecond)"`
	ExpiryMin  int64     `parquet:"expiry_min"`
	WindowOpen time.Time `parquet:"window_open,timestamp(nanosecond)"`
	PriceUp    float64   `parquet:"price_up"`
	Size       int16     `parquet:"size"`
	Aggressor  string    `parquet:"aggressor"`
}

type OptWindowRow struct {
	ExpiryMin   int64     `parquet:"expiry_min"`
	WindowOpen  time.Time `parquet:"window_open,timestamp(nanosecond)"`
	Strike      float32   `parquet:"strike"`
	Settle      float32   `parquet:"settle"`
	UpWon       bool      `parquet:"up_won"`
	PriceSource string    `parquet:"price_source"`
}

var dataTables = []string{"top_of_book", "trades", "option_top_of_book", "option_trades", "option_windows"}

func secTime(sec int64) time.Time { return time.Unix(sec, 0).UTC() }
func dateOf(sec int64) string     { return secTime(sec).Format("2006-01-02") }

func clampSize(q int) int16 {
	if q > 32767 { return 32767 }
	return int16(q)
}

type Recorder struct {
	mu      sync.Mutex
	dir     string
	date    string
	dirty   bool
	tob     []TobRow
	trades  []TradeRow
	otob    []OptTobRow
	otrades []OptTradeRow
	windows []OptWindowRow
}

func NewRecorder(dir string) *Recorder {
	r := &Recorder{dir: dir, date: dateOf(time.Now().Unix())}
	r.tob = readRows[TobRow](r.path("top_of_book", r.date))
	r.trades = readRows[TradeRow](r.path("trades", r.date))
	r.otob = readRows[OptTobRow](r.path("option_top_of_book", r.date))
	r.otrades = readRows[OptTradeRow](r.path("option_trades", r.date))
	r.windows = readRows[OptWindowRow](r.path("option_windows", r.date))
	return r
}

func readRows[T any](path string) []T {
	if _, err := os.Stat(path); err != nil { return nil }
	rows, err := parquet.ReadFile[T](path)
	if err != nil {
		log.Printf("recorder: could not read %s: %v", path, err)
		return nil
	}
	return rows
}

func (r *Recorder) path(table, date string) string {
	return filepath.Join(r.dir, table, "date="+date+".parquet")
}

// roll starts a new day when sec falls on a later date. Caller holds r.mu.
func (r *Recorder) roll(sec int64) {
	d := dateOf(sec)
	if d <= r.date { return }
	r.writeLocked()
	r.date = d
	r.tob, r.trades, r.otob, r.otrades, r.windows = nil, nil, nil, nil, nil
}

func (r *Recorder) addTob(rows []TobRow, orows []OptTobRow, sec int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roll(sec)
	r.tob = append(r.tob, rows...)
	r.otob = append(r.otob, orows...)
	r.dirty = true
}

func (r *Recorder) addTrade(sec int64, product string, price float64, size int, aggressor string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roll(sec)
	r.trades = append(r.trades, TradeRow{Ts: secTime(sec), Product: product, Price: price, Size: clampSize(size), Aggressor: aggressor})
	r.dirty = true
}

func (r *Recorder) addOptionTrade(sec int64, expiry int, windowOpen int64, price float64, size int, aggressor string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roll(sec)
	r.otrades = append(r.otrades, OptTradeRow{
		Ts: secTime(sec), ExpiryMin: int64(expiry), WindowOpen: secTime(windowOpen),
		PriceUp: price, Size: clampSize(size), Aggressor: aggressor,
	})
	r.dirty = true
}

func (r *Recorder) addWindow(w OptionWindow, closeSec int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roll(closeSec)
	r.windows = append(r.windows, OptWindowRow{
		ExpiryMin: int64(w.ExpiryMin), WindowOpen: secTime(w.WindowOpen),
		Strike: float32(w.Strike), Settle: float32(w.Settle), UpWon: w.UpWon, PriceSource: w.PriceSource,
	})
	r.dirty = true
}

// Flush writes the current day's files if anything changed. Rows are only ever appended,
// so the slices captured under the lock stay valid while writing.
func (r *Recorder) Flush() {
	r.mu.Lock()
	if !r.dirty {
		r.mu.Unlock()
		return
	}
	r.dirty = false
	date, tob, trades, otob, otrades, windows := r.date, r.tob, r.trades, r.otob, r.otrades, r.windows
	r.mu.Unlock()
	r.write(date, tob, trades, otob, otrades, windows)
}

func (r *Recorder) writeLocked() {
	if !r.dirty { return }
	r.dirty = false
	r.write(r.date, r.tob, r.trades, r.otob, r.otrades, r.windows)
}

func (r *Recorder) write(date string, tob []TobRow, trades []TradeRow, otob []OptTobRow, otrades []OptTradeRow, windows []OptWindowRow) {
	writeRows(r.path("top_of_book", date), tob)
	writeRows(r.path("trades", date), trades)
	writeRows(r.path("option_top_of_book", date), otob)
	writeRows(r.path("option_trades", date), otrades)
	writeRows(r.path("option_windows", date), windows)
}

func writeRows[T any](path string, rows []T) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("recorder: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err := parquet.WriteFile(tmp, rows); err != nil {
		log.Printf("recorder: write %s: %v", path, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil { log.Printf("recorder: %v", err) }
}

type DataFile struct {
	Table string `json:"table"`
	Date  string `json:"date"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func (r *Recorder) Files() []DataFile {
	out := make([]DataFile, 0)
	for _, t := range dataTables {
		matches, _ := filepath.Glob(filepath.Join(r.dir, t, "date=*.parquet"))
		for _, m := range matches {
			st, err := os.Stat(m)
			if err != nil { continue }
			base := filepath.Base(m)
			date := base[len("date=") : len(base)-len(".parquet")]
			out = append(out, DataFile{Table: t, Date: date, Path: t + "/" + base, Bytes: st.Size()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date { return out[i].Date > out[j].Date }
		return out[i].Table < out[j].Table
	})
	return out
}

var dataPathRe = regexp.MustCompile(`^([a-z_]+)/date=(\d{4}-\d{2}-\d{2})\.parquet$`)

// Resolve validates a public data path and returns the file on disk.
func (r *Recorder) Resolve(rel string) (string, error) {
	m := dataPathRe.FindStringSubmatch(rel)
	if m == nil { return "", fmt.Errorf("bad path") }
	ok := false
	for _, t := range dataTables {
		if t == m[1] { ok = true }
	}
	if !ok { return "", fmt.Errorf("unknown table") }
	r.mu.Lock()
	today := r.date
	r.mu.Unlock()
	if m[2] == today { r.Flush() }
	p := r.path(m[1], m[2])
	if _, err := os.Stat(p); err != nil { return "", fmt.Errorf("not found") }
	return p, nil
}

// recordTopOfBook snapshots best bid/ask for every product at the end of second sec.
// Caller holds ex.mu.
func (ex *Exchange) recordTopOfBook(sec int64) {
	type top struct {
		bid, ask       float64
		bidQ, askQ     int
		hasBid, hasAsk bool
	}
	tops := make(map[string]*top)
	for _, o := range ex.Orders {
		rem := o.Remaining()
		if rem <= 0 { continue }
		t := tops[o.ProductID]
		if t == nil {
			t = &top{}
			tops[o.ProductID] = t
		}
		if o.Side == "buy" {
			if !t.hasBid || o.Price > t.bid+1e-9 {
				t.bid, t.bidQ, t.hasBid = o.Price, rem, true
			} else if math.Abs(o.Price-t.bid) < 1e-9 {
				t.bidQ += rem
			}
		} else {
			if !t.hasAsk || o.Price < t.ask-1e-9 {
				t.ask, t.askQ, t.hasAsk = o.Price, rem, true
			} else if math.Abs(o.Price-t.ask) < 1e-9 {
				t.askQ += rem
			}
		}
	}
	fill := func(t *top) (*float64, *float64, int16, int16) {
		if t == nil { return nil, nil, 0, 0 }
		var b, a *float64
		if t.hasBid { v := t.bid; b = &v }
		if t.hasAsk { v := t.ask; a = &v }
		return b, a, clampSize(t.bidQ), clampSize(t.askQ)
	}

	ts := secTime(sec)
	var rows []TobRow
	for _, sym := range append(append([]string{}, fruitSymbols...), EtfSymbol) {
		pid, ok := ex.Symbols[sym]
		if !ok { continue }
		b, a, bq, aq := fill(tops[pid])
		rows = append(rows, TobRow{Ts: ts, Product: sym, Bid: b, Ask: a, BidSize: bq, AskSize: aq})
	}
	var orows []OptTobRow
	for _, e := range allExpiries {
		os := ex.Options[e]
		if os == nil || !ex.Config.expiryEnabled(e) { continue }
		b, a, bq, aq := fill(tops[os.ProductID])
		orows = append(orows, OptTobRow{
			Ts: ts, ExpiryMin: int16(e), WindowOpen: secTime(os.WindowOpen),
			Bid: b, Ask: a, BidSize: bq, AskSize: aq,
		})
	}
	ex.rec.addTob(rows, orows, sec)
}

