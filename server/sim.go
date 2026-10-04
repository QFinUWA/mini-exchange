package main

// Market simulator: hidden fair value processes and the house bots that trade on them.
// Organiser-only. See SIMULATION.md. Parameters are calibrated to the handout data
// (spreads, top-of-book sizes, trade rates and sizes, volatility at 1s/1m/1h).

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"time"
)

var simRng = func() *rand.Rand {
	var b [16]byte
	cryptorand.Read(b[:])
	return rand.New(rand.NewPCG(binary.LittleEndian.Uint64(b[:8]), binary.LittleEndian.Uint64(b[8:])))
}()

const simTickSec = 0.25

type SimState struct {
	FV         map[string]float64 `json:"fv"`
	AppleMu    float64            `json:"appleMu"`
	AppleDiffs []float64          `json:"appleDiffs"`
	BananaVel  float64            `json:"bananaVel"`
	StrawRev   float64            `json:"strawRev"`
	MelonBase  float64            `json:"melonBase"`
	MMRef      map[string]float64 `json:"mmRef"`
	MMPos      map[string]int     `json:"mmPos"`
	OptOffset  map[string]float64 `json:"optOffset"`
	OptWindow  map[string]int64   `json:"optWindow"`
}

type bookParams struct {
	anchor      float64 // long run level of the fair value
	half        float64 // house MM half spread
	step        float64 // distance between MM levels
	sizes       []int   // MM size per level
	lambda      float64 // MM reference move per unit of its own inventory change
	skew        float64 // MM quote shift per unit of inventory
	noiseRate   float64 // aggressive noise orders per second
	sizeMed     float64 // noise order size median
	sizeSig     float64 // noise order size log sd
	sizeMax     int
	passiveRate float64 // resting noise orders per second
	infRate     float64 // informed checks per second
	infThresh   float64 // informed trade when |mid - fv| exceeds this
	infMax      int
}

func fruitParams(anchor float64) bookParams {
	return bookParams{
		anchor: anchor, half: 0.03, step: 0.01, sizes: []int{8, 14, 22},
		lambda: 0.0006, skew: 0.0002,
		noiseRate: 0.6, sizeMed: 10, sizeSig: 0.86, sizeMax: 300, passiveRate: 0.15,
		infRate: 0.3, infThresh: 0.05, infMax: 40,
	}
}

var simParams = map[string]bookParams{
	"apples":       fruitParams(11),
	"bananas":      fruitParams(9),
	"oranges":      fruitParams(12),
	"strawberries": fruitParams(10),
	"watermelon":   fruitParams(13.5),
	EtfSymbol: {
		anchor: 55.5, half: 0.07, step: 0.02, sizes: []int{2, 4, 8},
		lambda: 0.004, skew: 0.001,
		noiseRate: 0.25, sizeMed: 3, sizeSig: 1.2, sizeMax: 170, passiveRate: 0.05,
		infRate: 0.15, infThresh: 0.12, infMax: 20,
	},
}

type optParams struct {
	noiseRate float64
	vol       float64 // naive ETF vol per sqrt(second) used by the house option MM
}

var optSimParams = map[int]optParams{
	5:  {noiseRate: 0.15, vol: 0.012},
	15: {noiseRate: 0.08, vol: 0.012},
	60: {noiseRate: 0.05, vol: 0.012},
}

func (ex *Exchange) ensureSim() {
	s := ex.Sim
	if s == nil {
		s = &SimState{}
		ex.Sim = s
	}
	if s.FV == nil {
		s.FV = make(map[string]float64)
		for _, sym := range fruitSymbols {
			s.FV[sym] = simParams[sym].anchor
		}
		s.AppleMu = simParams["apples"].anchor
		s.MelonBase = simParams["watermelon"].anchor
	}
	if s.MMRef == nil { s.MMRef = make(map[string]float64) }
	if s.MMPos == nil { s.MMPos = make(map[string]int) }
	if s.OptOffset == nil { s.OptOffset = make(map[string]float64) }
	if s.OptWindow == nil { s.OptWindow = make(map[string]int64) }
	for sym := range simParams {
		if _, ok := s.MMRef[sym]; !ok { s.MMRef[sym] = ex.fairValue(sym) }
	}
}

func (ex *Exchange) fairValue(sym string) float64 {
	if sym == EtfSymbol {
		t := 0.0
		for _, f := range fruitSymbols { t += ex.Sim.FV[f] }
		return t
	}
	return ex.Sim.FV[sym]
}

// simSecond advances every fruit's fair value by one second.
func (ex *Exchange) simSecond(sec int64) {
	s := ex.Sim
	n := simRng.NormFloat64
	pull := func(x, anchor float64) float64 { return -(x - anchor) / 20000 }

	// apples: mean reverts to a slowly wandering level.
	s.AppleMu += 0.0008*n() + pull(s.AppleMu, simParams["apples"].anchor)*2.5
	a := s.FV["apples"]
	na := a + (s.AppleMu-a)/900 + 0.006*n()
	s.FV["apples"] = na
	s.AppleDiffs = append(s.AppleDiffs, na-a)
	if len(s.AppleDiffs) > 30 { s.AppleDiffs = s.AppleDiffs[len(s.AppleDiffs)-30:] }

	// bananas: persistent drift (trends).
	s.BananaVel = 0.98*s.BananaVel + 0.0001*n()
	b := s.FV["bananas"]
	s.FV["bananas"] = b + s.BananaVel + 0.003*n() + pull(b, simParams["bananas"].anchor)

	// oranges: follow apples with a 20 second lag.
	o := s.FV["oranges"]
	lagged := 0.0
	if len(s.AppleDiffs) >= 21 { lagged = s.AppleDiffs[len(s.AppleDiffs)-21] }
	s.FV["oranges"] = o + lagged + 0.004*n() + pull(o, simParams["oranges"].anchor)

	// strawberries: rare jumps that half revert over the next few minutes.
	st := s.FV["strawberries"]
	if simRng.Float64() < 1.0/900 {
		j := 0.25 * n()
		st += j
		s.StrawRev -= 0.5 * j
	}
	rev := s.StrawRev / 60
	st += rev
	s.StrawRev -= rev
	s.FV["strawberries"] = st + 0.004*n() + pull(st, simParams["strawberries"].anchor)

	// watermelon: random walk plus a 15 minute cycle.
	s.MelonBase += 0.007*n() + pull(s.MelonBase, simParams["watermelon"].anchor)
	s.FV["watermelon"] = s.MelonBase + 0.25*math.Sin(2*math.Pi*float64(sec%900)/900)

	for _, f := range fruitSymbols {
		if s.FV[f] < 1 { s.FV[f] = 1 }
	}
}

// SimTick runs the house bots. Called every simTickSec.
func (ex *Exchange) SimTick() {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if !ex.IsOpen || !ex.Config.SimEnabled || ex.Sim == nil { return }

	now := time.Now()
	mm, flow, inf, opt := ex.houseID("house-mm"), ex.houseID("house-flow"), ex.houseID("house-informed"), ex.houseID("house-options")
	ex.cancelStale(flow, now.UnixMilli()-30000)

	for _, sym := range append(append([]string{}, fruitSymbols...), EtfSymbol) {
		pid, ok := ex.Symbols[sym]
		if !ok { continue }
		p := ex.Products[pid]
		prm := simParams[sym]
		ex.simMarketMake(mm, p, prm)
		ex.simNoise(flow, p, prm)
		ex.simInformed(inf, p, prm, ex.fairValue(sym))
	}

	for _, e := range allExpiries {
		os := ex.Options[e]
		if os == nil || !ex.Config.expiryEnabled(e) || os.Strike == nil { continue }
		ex.simOptions(opt, flow, inf, os, now)
	}

	ex.saveAsync()
	ex.doBroadcast()
}

func (ex *Exchange) posQty(userID, productID string) int {
	if pos := ex.Positions[posKey(userID, productID)]; pos != nil { return pos.Qty }
	return 0
}

func (ex *Exchange) cancelUserProduct(userID, productID string) {
	for id, o := range ex.Orders {
		if o.UserID == userID && o.ProductID == productID { delete(ex.Orders, id) }
	}
}

func (ex *Exchange) cancelStale(userID string, beforeMs int64) {
	for id, o := range ex.Orders {
		if o.UserID == userID && o.CreatedAt < beforeMs { delete(ex.Orders, id) }
	}
}

func (ex *Exchange) bestPrices(productID string) (bid, ask float64, hasBid, hasAsk bool) {
	for _, o := range ex.Orders {
		if o.ProductID != productID || o.Remaining() <= 0 { continue }
		if o.Side == "buy" && (!hasBid || o.Price > bid) { bid, hasBid = o.Price, true }
		if o.Side == "sell" && (!hasAsk || o.Price < ask) { ask, hasAsk = o.Price, true }
	}
	return
}

func (ex *Exchange) houseOrder(userID string, p *Product, side string, price float64, qty int, orderType string) {
	price = round2(roundTo(price, p.TickSize))
	if p.kind() == KindOption {
		price = math.Min(0.99, math.Max(0.01, price))
	}
	if price <= 0 || qty <= 0 { return }
	ex.placeOrderInternal(userID, p.ID, side, price, qty, orderType)
}

func lognormalSize(med, sig float64, max int) int {
	q := int(math.Round(med * math.Exp(sig*simRng.NormFloat64())))
	if q < 1 { q = 1 }
	if q > max { q = max }
	return q
}

func chance(ratePerSec float64) bool { return simRng.Float64() < ratePerSec*simTickSec }

// simMarketMake: uninformed MM. Its reference price only moves with the flow it trades against.
func (ex *Exchange) simMarketMake(mm string, p *Product, prm bookParams) {
	s := ex.Sim
	pos := ex.posQty(mm, p.ID)
	if d := pos - s.MMPos[p.ID]; d != 0 {
		s.MMRef[p.Symbol] -= prm.lambda * float64(d)
	}
	s.MMPos[p.ID] = pos
	if s.MMRef[p.Symbol] < 1 { s.MMRef[p.Symbol] = 1 }

	ref := s.MMRef[p.Symbol] - prm.skew*float64(pos)
	bid := math.Floor((ref-prm.half)/p.TickSize+1e-9) * p.TickSize
	ask := math.Ceil((ref+prm.half)/p.TickSize-1e-9) * p.TickSize
	ex.cancelUserProduct(mm, p.ID)
	for i, size := range prm.sizes {
		off := float64(i) * prm.step
		ex.houseOrder(mm, p, "buy", bid-off, size, "limit")
		ex.houseOrder(mm, p, "sell", ask+off, size, "limit")
	}
}

// simNoise: uninformed flow. Mostly marketable orders, some resting orders near the touch.
func (ex *Exchange) simNoise(flow string, p *Product, prm bookParams) {
	if chance(prm.noiseRate) {
		bid, ask, hb, ha := ex.bestPrices(p.ID)
		size := lognormalSize(prm.sizeMed, prm.sizeSig, prm.sizeMax)
		if simRng.IntN(2) == 0 && ha {
			ex.houseOrder(flow, p, "buy", ask+10*p.TickSize, size, "ioc")
		} else if hb {
			ex.houseOrder(flow, p, "sell", bid-10*p.TickSize, size, "ioc")
		}
	}
	if chance(prm.passiveRate) {
		if mid := ex.getMidPrice(p.ID); mid != nil {
			off := float64(1+simRng.IntN(4)) * prm.step
			size := lognormalSize(prm.sizeMed, prm.sizeSig, prm.sizeMax)
			if simRng.IntN(2) == 0 {
				ex.houseOrder(flow, p, "buy", *mid-prm.half-off, size, "limit")
			} else {
				ex.houseOrder(flow, p, "sell", *mid+prm.half+off, size, "limit")
			}
		}
	}
}

// simInformed: sees a noisy fair value and trades the price back towards it.
func (ex *Exchange) simInformed(inf string, p *Product, prm bookParams, fv float64) {
	if !chance(prm.infRate) { return }
	mid := ex.getMidPrice(p.ID)
	if mid == nil { return }
	obs := fv + 0.3*prm.half*simRng.NormFloat64()
	dev := *mid - obs
	if math.Abs(dev) <= prm.infThresh { return }
	size := int(math.Ceil(5 * math.Abs(dev) / prm.half))
	if size > prm.infMax { size = prm.infMax }
	if dev > 0 {
		ex.houseOrder(inf, p, "sell", obs+0.3*prm.half, size, "ioc")
	} else {
		ex.houseOrder(inf, p, "buy", obs-0.3*prm.half, size, "ioc")
	}
}

func normCdf(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// simOptions: a naive house MM (normal model on the ETF mid, fixed vol), noise flow and a
// weak informed trader that knows the ETF fair value.
func (ex *Exchange) simOptions(mm, flow, inf string, os *OptionState, now time.Time) {
	s := ex.Sim
	p := ex.Products[os.ProductID]
	prm := optSimParams[os.ExpiryMin]
	etf := ex.etfProduct()
	if p == nil || etf == nil { return }
	spot := ex.getMidPrice(etf.ID)
	if spot == nil { return }

	if s.OptWindow[p.Symbol] != os.WindowOpen {
		s.OptWindow[p.Symbol] = os.WindowOpen
		s.OptOffset[p.Symbol] = 0
		s.MMPos[p.ID] = ex.posQty(mm, p.ID)
	}

	tau := float64(os.close()+1) - float64(now.UnixMilli())/1000
	if tau < 0.5 { tau = 0.5 }
	prob := func(x, vol float64) float64 { return normCdf((x - *os.Strike) / (vol * math.Sqrt(tau))) }

	pos := ex.posQty(mm, p.ID)
	if d := pos - s.MMPos[p.ID]; d != 0 { s.OptOffset[p.Symbol] -= 0.0008 * float64(d) }
	s.MMPos[p.ID] = pos
	s.OptOffset[p.Symbol] *= 0.995

	q := prob(*spot, prm.vol) + s.OptOffset[p.Symbol] - 0.0002*float64(pos)
	bid := math.Floor((q-0.035)*100+1e-9) / 100
	ask := bid + 0.07
	if bid < 0.01 { bid, ask = 0.01, 0.08 }
	if ask > 0.99 { bid, ask = 0.92, 0.99 }
	ex.cancelUserProduct(mm, p.ID)
	ex.houseOrder(mm, p, "buy", bid, 50, "limit")
	ex.houseOrder(mm, p, "sell", ask, 50, "limit")
	if bid-0.03 >= 0.01 { ex.houseOrder(mm, p, "buy", bid-0.03, 100, "limit") }
	if ask+0.03 <= 0.99 { ex.houseOrder(mm, p, "sell", ask+0.03, 100, "limit") }

	if chance(prm.noiseRate) {
		b, a, hb, ha := ex.bestPrices(p.ID)
		size := lognormalSize(11, 0.8, 100)
		if simRng.IntN(2) == 0 && ha {
			ex.houseOrder(flow, p, "buy", a+0.05, size, "ioc")
		} else if hb {
			ex.houseOrder(flow, p, "sell", b-0.05, size, "ioc")
		}
	}

	if chance(0.02) {
		mid := ex.getMidPrice(p.ID)
		if mid == nil { return }
		fair := prob(ex.fairValue(EtfSymbol), prm.vol*(0.8+0.4*simRng.Float64()))
		if dev := *mid - fair; math.Abs(dev) > 0.08 {
			if dev > 0 {
				ex.houseOrder(inf, p, "sell", fair+0.03, 20, "ioc")
			} else {
				ex.houseOrder(inf, p, "buy", fair-0.03, 20, "ioc")
			}
		}
	}
}
