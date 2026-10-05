package main

// Backtest: run one uploaded bot against the simulated market for many trading days, as fast as
// the bot can go. The market (house bots, fair values, matching, margin, days, options) is the
// live exchange code running on a simulated clock. The bot is the normal SDK bot in a Python
// subprocess; the SDK sees QFIN_BACKTEST=1 and talks over fds 3 and 4 instead of HTTP/WS:
//
//   exchange -> bot (fd 3): {"type":"tick","state":{...}}   once per simulated second
//                           {"type":"resp","status":200,"body":...}   answer to a call
//                           {"type":"end"}
//   bot -> exchange (fd 4): {"type":"ready"}                once, before the first tick
//                           {"type":"call","method":"POST","path":"/api/batch","body":{...}}
//                           {"type":"done"}                 on_tick returned
//
// Calls go through the real HTTP handlers (newMux), so the bot sees exactly the live API.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Simulated days start at this UTC midnight (2026-01-01).
const backtestEpoch = 1767225600

type BacktestParams struct {
	Days    int    `json:"days"`
	DayMin  int    `json:"dayMin"`
	Seed    uint64 `json:"seed"`
	Warmup  int    `json:"warmupSec"`
}

type BacktestDay struct {
	Day   string  `json:"day"`
	Pnl   float64 `json:"pnl"`
	Busts int     `json:"busts"`
}

type BacktestResult struct {
	Status       string         `json:"status"` // "done" or "error"
	Error        string         `json:"error,omitempty"`
	Params       BacktestParams `json:"params"`
	Days         []BacktestDay  `json:"days"`
	MeanDailyPnl float64        `json:"meanDailyPnl"`
	StdDailyPnl  float64        `json:"stdDailyPnl"`
	Score        float64        `json:"score"`
	Busts        int            `json:"busts"`
	Fills        int            `json:"fills"`
	Volume       int            `json:"volume"`
	WallSec      float64        `json:"wallSec"`
	Log          string         `json:"log"`
}

type BotLimits struct {
	Python      string        // interpreter
	SdkDir      string        // put on PYTHONPATH (the official exchange.py)
	UID         int           // run the bot as this uid/gid (0 = don't switch)
	MemMB       int           // address space limit via prlimit (0 = none)
	Timeout     time.Duration // whole run, wall clock
	TickTimeout time.Duration // one on_tick, wall clock
	MaxCalls    int           // API calls per tick
}

// botProc is the bot subprocess and its protocol pipes.
type botProc struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	lines chan []byte
	logs  *capLog
	exit  chan error
}

type botMsg struct {
	Type   string          `json:"type"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

// capLog keeps the first and last part of the bot's stdout/stderr.
type capLog struct {
	mu         sync.Mutex
	head, tail []byte
	dropped    int
}

const logHead, logTail = 16 << 10, 48 << 10

func (c *capLog) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	if room := logHead - len(c.head); room > 0 {
		k := min(room, len(p))
		c.head = append(c.head, p[:k]...)
		p = p[k:]
	}
	c.tail = append(c.tail, p...)
	if over := len(c.tail) - logTail; over > 0 {
		c.dropped += over
		c.tail = append(c.tail[:0], c.tail[over:]...)
	}
	return n, nil
}

func (c *capLog) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dropped == 0 { return string(c.head) + string(c.tail) }
	return fmt.Sprintf("%s\n... (%d bytes of output skipped) ...\n%s", c.head, c.dropped, c.tail)
}

func startBot(botFile string, lim BotLimits) (*botProc, error) {
	toBotR, toBotW, err := os.Pipe()
	if err != nil { return nil, err }
	fromBotR, fromBotW, err := os.Pipe()
	if err != nil { return nil, err }

	dir := filepath.Dir(botFile)
	args := []string{lim.Python, "-u", filepath.Base(botFile)}
	if lim.MemMB > 0 {
		if pl, err := exec.LookPath("prlimit"); err == nil {
			args = append([]string{pl, fmt.Sprintf("--as=%d", lim.MemMB<<20), "--nproc=64",
				"--nofile=256", fmt.Sprintf("--fsize=%d", 50<<20), "--"}, args...)
		}
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir, "PYTHONPATH=" + lim.SdkDir,
		"PYTHONDONTWRITEBYTECODE=1", "QFIN_BACKTEST=1",
		"OMP_NUM_THREADS=1", "OPENBLAS_NUM_THREADS=1", "MKL_NUM_THREADS=1",
	}
	logs := &capLog{}
	cmd.Stdout, cmd.Stderr = logs, logs
	cmd.ExtraFiles = []*os.File{toBotR, fromBotW} // fd 3, fd 4
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A bot that forks a detached child keeps our pipes open; don't let that block Wait.
	cmd.WaitDelay = 2 * time.Second
	if lim.UID > 0 {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(lim.UID), Gid: uint32(lim.UID)}
	}
	if err := cmd.Start(); err != nil { return nil, err }
	toBotR.Close()
	fromBotW.Close()

	b := &botProc{cmd: cmd, in: toBotW, lines: make(chan []byte, 1), logs: logs, exit: make(chan error, 1)}
	go func() {
		r := bufio.NewReaderSize(fromBotR, 1<<20)
		for {
			line, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 { b.lines <- line }
			if err != nil { close(b.lines); return }
		}
	}()
	go func() { b.exit <- cmd.Wait() }()
	return b, nil
}

func (b *botProc) send(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil { return err }
	_, err = b.in.Write(append(data, '\n'))
	return err
}

// recv waits for the next message from the bot.
func (b *botProc) recv(timeout time.Duration) (*botMsg, error) {
	select {
	case line, ok := <-b.lines:
		if !ok { return nil, fmt.Errorf("bot exited") }
		var m botMsg
		if err := json.Unmarshal(line, &m); err != nil { return nil, fmt.Errorf("bad message from bot: %.200s", line) }
		return &m, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("bot did not answer within %v", timeout)
	}
}

func (b *botProc) kill(uid int) {
	b.in.Close()
	select {
	case <-b.exit:
	case <-time.After(2 * time.Second):
		syscall.Kill(-b.cmd.Process.Pid, syscall.SIGKILL)
		b.cmd.Process.Kill()
		<-b.exit
	}
	killUID(uid) // anything it forked into its own session
}

// Paths a bot may call in the backtest (prefix match). Everything else is 404.
var backtestPaths = []string{
	"/api/order", "/api/cancel-all", "/api/batch", "/api/etf-swap", "/api/state", "/api/config",
	"/api/option-windows", "/api/products", "/api/exchange-state", "/api/trades",
}

func backtestAllowed(path string) bool {
	p := strings.SplitN(path, "?", 2)[0]
	for _, a := range backtestPaths {
		if p == a { return true }
	}
	return false
}

// serveCall runs one bot API call through the real handlers.
func serveCall(mux *http.ServeMux, token string, m *botMsg) map[string]interface{} {
	if !backtestAllowed(m.Path) {
		return map[string]interface{}{"status": 404, "body": map[string]string{"error": m.Path + " is not available in the backtest"}}
	}
	method := strings.ToUpper(m.Method)
	if method == "" { method = "GET" }
	var body io.Reader
	if len(m.Body) > 0 && string(m.Body) != "null" { body = bytes.NewReader(m.Body) }
	req := httptest.NewRequest(method, m.Path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil { out = rec.Body.String() }
	return map[string]interface{}{"status": rec.Code, "body": out}
}

// RunBacktest simulates p.Days trading days with the bot in botFile.
func RunBacktest(botFile string, p BacktestParams, lim BotLimits) *BacktestResult {
	res := &BacktestResult{Params: p, Days: []BacktestDay{}}
	fail := func(format string, a ...interface{}) *BacktestResult {
		res.Status, res.Error = "error", fmt.Sprintf(format, a...)
		return res
	}
	if p.Days < 1 || p.DayMin < 1 { return fail("days and dayMin must be >= 1") }

	dayLen := int64(p.DayMin) * 60
	t0 := (backtestEpoch + dayLen - 1) / dayLen * dayLen // first bot day starts on a day boundary
	start := t0 - int64(p.Warmup)
	end := t0 + int64(p.Days)*dayLen

	var vms int64 = start * 1000
	seedSim(p.Seed)
	ex := NewExchange("")
	ex.clock = func() time.Time { return time.UnixMilli(vms) }
	exchange = ex // the HTTP handlers use the global
	mux := newMux()

	ex.mu.Lock()
	ex.Config = defaultConfig()
	ex.Config.DayLengthMin = p.DayMin
	ex.ensureProject()
	ex.DayStart = start
	ex.IsOpen = true
	ex.mu.Unlock()

	u, err := ex.Register("bot", randomHex(8))
	if err != nil { return fail("register: %v", err) }
	ex.mu.Lock()
	u.IsAdmin = false
	u.RateLimitMs = 1 // the simulated clock moves 1 ms per call
	ex.mu.Unlock()

	bot, err := startBot(botFile, lim)
	if err != nil { return fail("could not start bot: %v", err) }
	defer func() { res.Log = bot.logs.String() }()
	defer bot.kill(lim.UID)

	if m, err := bot.recv(60 * time.Second); err != nil || m.Type != "ready" {
		if err == nil { err = fmt.Errorf("expected ready, got %q", m.Type) }
		return fail("bot did not start: %v (does it call .run()?)", err)
	}

	ex.onFill = func(f *Fill) {
		if f.BuyUserID == u.ID || f.SellUserID == u.ID {
			res.Fills++
			res.Volume += f.Qty
		}
	}

	collect := func() {
		res.Days = res.Days[:0]
		for _, r := range ex.DailyResults {
			if r.UserID == u.ID { res.Days = append(res.Days, BacktestDay{Day: r.Day, Pnl: r.Pnl, Busts: r.Busts}) }
		}
	}
	defer collect()

	deadline := time.Now().Add(lim.Timeout)
	for t := start; t < end; t++ {
		vms = t*1000 + 2
		if t > start { ex.OnSecond(t - 1) }

		if t >= t0 {
			if time.Now().After(deadline) {
				return fail("time limit: the backtest took longer than %v (simulated up to %s)", lim.Timeout, time.Unix(t, 0).UTC().Format("2006-01-02 15:04"))
			}
			vms = t*1000 + 5
			ex.mu.RLock()
			state := ex.getBotStateInternal(u.ID)
			ex.mu.RUnlock()
			if err := bot.send(map[string]interface{}{"type": "tick", "state": state}); err != nil {
				return fail("bot exited (%v)", err)
			}
			calls := 0
			for {
				m, err := bot.recv(lim.TickTimeout)
				if err != nil { return fail("on_tick at %s: %v", time.Unix(t, 0).UTC().Format("2006-01-02 15:04:05"), err) }
				if m.Type == "done" { break }
				if m.Type != "call" { return fail("unexpected message %q from bot", m.Type) }
				var reply map[string]interface{}
				if calls >= lim.MaxCalls {
					reply = map[string]interface{}{"status": 429, "body": map[string]string{"error": fmt.Sprintf("too many requests this tick (max %d)", lim.MaxCalls)}}
				} else {
					calls++
					vms++
					reply = serveCall(mux, u.SessionToken, m)
				}
				reply["type"] = "resp"
				if err := bot.send(reply); err != nil { return fail("bot exited (%v)", err) }
			}
		}

		for k := int64(0); k < 4; k++ {
			vms = t*1000 + 125 + 250*k
			ex.SimTick()
		}
	}
	vms = end*1000 + 2
	ex.OnSecond(end - 1) // ends the last day
	bot.send(map[string]string{"type": "end"})

	collect()
	res.Status = "done"
	return res
}

func (r *BacktestResult) finish(wallStart time.Time) {
	r.WallSec = math.Round(time.Since(wallStart).Seconds()*10) / 10
	r.Busts, r.MeanDailyPnl, r.StdDailyPnl, r.Score = 0, 0, 0, 0
	n := float64(len(r.Days))
	if n == 0 { return }
	for _, d := range r.Days {
		r.MeanDailyPnl += d.Pnl
		r.Busts += d.Busts
	}
	r.MeanDailyPnl /= n
	for _, d := range r.Days {
		r.StdDailyPnl += (d.Pnl - r.MeanDailyPnl) * (d.Pnl - r.MeanDailyPnl)
	}
	r.StdDailyPnl = math.Sqrt(r.StdDailyPnl / n)
	r.Score = r.MeanDailyPnl - r.StdDailyPnl
}

func runBacktestCmd(args []string) {
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	bot := fs.String("bot", "", "bot .py file (its folder is the working directory)")
	days := fs.Int("days", 5, "simulated trading days")
	dayMin := fs.Int("day-min", 1440, "minutes per trading day")
	seed := fs.Uint64("seed", 1, "market seed (same seed = same fair value paths)")
	warmup := fs.Int("warmup", 600, "simulated seconds of market before the bot starts")
	python := fs.String("python", "python3", "python interpreter")
	sdk := fs.String("sdk", "", "folder with the SDK exchange.py (default: ../bot-sdk next to the bot, else the bot folder)")
	uid := fs.Int("uid", 0, "run the bot as this uid/gid (needs root)")
	mem := fs.Int("mem-mb", 0, "address space limit for the bot in MB (0 = none)")
	timeout := fs.Duration("timeout", 30*time.Minute, "wall clock limit for the whole run")
	tickTimeout := fs.Duration("tick-timeout", 5*time.Second, "wall clock limit for one on_tick")
	maxCalls := fs.Int("max-calls", 100, "API calls allowed per tick")
	out := fs.String("out", "", "write the result JSON here (default stdout)")
	fs.Parse(args)
	if *bot == "" { log.Fatal("-bot is required") }
	botFile, _ := filepath.Abs(*bot)
	if *sdk == "" { *sdk = filepath.Dir(botFile) }
	sdkDir, _ := filepath.Abs(*sdk)

	wallStart := time.Now()
	r := RunBacktest(botFile, BacktestParams{Days: *days, DayMin: *dayMin, Seed: *seed, Warmup: *warmup}, BotLimits{
		Python: *python, SdkDir: sdkDir, UID: *uid, MemMB: *mem,
		Timeout: *timeout, TickTimeout: *tickTimeout, MaxCalls: *maxCalls,
	})
	r.finish(wallStart)

	data, _ := json.MarshalIndent(r, "", "  ")
	if *out == "" {
		os.Stdout.Write(append(data, '\n'))
	} else if err := os.WriteFile(*out, data, 0600); err != nil {
		log.Fatal(err)
	}
}
