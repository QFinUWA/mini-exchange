package main

// Bot submissions: teams upload a bot, the runner (runner.go, separate sandboxed container)
// backtests it, and the result is scored like the live game (mean - std of daily P&L).
//
// Everything lives in a folder shared with the runner, one folder per submission:
//   bots/<id>/job.json     written here once (who, when, file, backtest params)
//   bots/<id>/bot.py|zip   the upload
//   bots/<id>/status.json  runner: started
//   bots/<id>/result.json  runner: BacktestResult

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxBotUpload = 5 << 20

var botsDir = "bots"

var submitMu sync.Mutex

var submissionID = regexp.MustCompile(`^s_[0-9]+_[0-9a-f]+$`)

type SubmissionJob struct {
	ID        string         `json:"id"`
	UserID    string         `json:"userId"`
	Username  string         `json:"username"`
	Filename  string         `json:"filename"`
	File      string         `json:"file"` // bot.py or bot.zip
	CreatedAt int64          `json:"createdAt"`
	Params    BacktestParams `json:"params"`
}

type Submission struct {
	SubmissionJob
	Status        string          `json:"status"` // queued, running, done, error
	QueuePosition int             `json:"queuePosition,omitempty"`
	StartedAt     int64           `json:"startedAt,omitempty"`
	Result        *BacktestResult `json:"result,omitempty"`
}

func readJSON(path string, v interface{}) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, v) == nil
}

// loadSubmissions reads every submission, oldest first.
func loadSubmissions(withLog bool) []*Submission {
	entries, _ := os.ReadDir(botsDir)
	var out []*Submission
	for _, e := range entries {
		if !e.IsDir() || !submissionID.MatchString(e.Name()) { continue }
		dir := filepath.Join(botsDir, e.Name())
		s := &Submission{Status: "queued"}
		if !readJSON(filepath.Join(dir, "job.json"), &s.SubmissionJob) { continue }
		var st struct{ StartedAt int64 `json:"startedAt"` }
		if readJSON(filepath.Join(dir, "status.json"), &st) {
			s.Status, s.StartedAt = "running", st.StartedAt
		}
		var r BacktestResult
		if readJSON(filepath.Join(dir, "result.json"), &r) {
			if !withLog { r.Log = "" }
			s.Status, s.Result = r.Status, &r
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	pos := 0
	for _, s := range out {
		if s.Status == "queued" { pos++; s.QueuePosition = pos }
	}
	return out
}

func (ex *Exchange) backtestParams() BacktestParams {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	c := ex.Config
	return BacktestParams{Days: c.backtestDays(), DayMin: c.backtestDayMin(), Seed: c.BacktestSeed, Warmup: 600}
}

// createSubmission stores an upload and queues it for the runner.
func createSubmission(u *User, filename string, data []byte, params BacktestParams) (*Submission, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".py" && ext != ".zip" { return nil, fmt.Errorf("upload a .py file, or a .zip with bot.py in it") }
	if len(data) == 0 { return nil, fmt.Errorf("empty file") }

	submitMu.Lock()
	defer submitMu.Unlock()
	for _, s := range loadSubmissions(false) {
		if s.UserID == u.ID && (s.Status == "queued" || s.Status == "running") {
			return nil, fmt.Errorf("your previous bot is still %s, wait for it to finish", s.Status)
		}
	}

	now := time.Now()
	job := SubmissionJob{
		ID: fmt.Sprintf("s_%d_%s", now.UnixMilli(), randomHex(3)), UserID: u.ID, Username: u.Username,
		Filename: filepath.Base(filename), File: "bot" + ext, CreatedAt: now.UnixMilli(), Params: params,
	}
	dir := filepath.Join(botsDir, job.ID)
	if err := os.MkdirAll(dir, 0700); err != nil { return nil, err }
	if err := os.WriteFile(filepath.Join(dir, job.File), data, 0600); err != nil { return nil, err }
	meta, _ := json.MarshalIndent(job, "", "  ")
	// job.json last: the runner only picks up folders that have it.
	if err := os.WriteFile(filepath.Join(dir, "job.json"), meta, 0600); err != nil { return nil, err }
	return &Submission{SubmissionJob: job, Status: "queued"}, nil
}

type BotLeaderboardEntry struct {
	Username     string         `json:"username"`
	SubmissionID string         `json:"submissionId"`
	Filename     string         `json:"filename"`
	SubmittedAt  int64          `json:"submittedAt"`
	Params       BacktestParams `json:"params"`
	Days         int            `json:"days"`
	MeanDailyPnl float64        `json:"meanDailyPnl"`
	StdDailyPnl  float64        `json:"stdDailyPnl"`
	Score        float64        `json:"score"`
	Busts        int            `json:"busts"`
}

// botLeaderboard ranks each team's latest successfully backtested bot.
func botLeaderboard() []BotLeaderboardEntry {
	latest := make(map[string]*Submission)
	for _, s := range loadSubmissions(false) {
		if s.Status == "done" { latest[s.UserID] = s }
	}
	out := make([]BotLeaderboardEntry, 0, len(latest))
	for _, s := range latest {
		r := s.Result
		out = append(out, BotLeaderboardEntry{
			Username: s.Username, SubmissionID: s.ID, Filename: s.Filename, SubmittedAt: s.CreatedAt,
			Params: s.Params, Days: len(r.Days), MeanDailyPnl: r.MeanDailyPnl, StdDailyPnl: r.StdDailyPnl,
			Score: r.Score, Busts: r.Busts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// publicSubmission hides the seed from non-admins.
func publicSubmission(s *Submission, isAdmin bool) *Submission {
	if isAdmin { return s }
	c := *s
	c.Params.Seed = 0
	if c.Result != nil {
		r := *c.Result
		r.Params.Seed = 0
		c.Result = &r
	}
	return &c
}

func addSubmissionRoutes(mux *http.ServeMux) {
	auth := func(w http.ResponseWriter, r *http.Request) *User {
		u, err := exchange.AuthenticateR(getToken(r))
		if err != nil { errResp(w, err.Error(), 401); return nil }
		return u
	}

	// POST: upload (multipart field "file"). GET: your submissions (admin: everyone's).
	mux.HandleFunc("/api/submissions", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		u := auth(w, r)
		if u == nil { return }

		switch r.Method {
		case "GET":
			out := make([]*Submission, 0)
			all := loadSubmissions(false)
			for i := len(all) - 1; i >= 0; i-- {
				if u.IsAdmin || all[i].UserID == u.ID { out = append(out, publicSubmission(all[i], u.IsAdmin)) }
			}
			jsonResp(w, out, 200)
		case "POST":
			r.Body = http.MaxBytesReader(w, r.Body, maxBotUpload+64<<10)
			f, hdr, err := r.FormFile("file")
			if err != nil { errResp(w, "send the bot as multipart field \"file\" (max 5 MB)", 400); return }
			defer f.Close()
			data, err := io.ReadAll(io.LimitReader(f, maxBotUpload+1))
			if err != nil || len(data) > maxBotUpload { errResp(w, "file too large (max 5 MB)", 400); return }
			s, err := createSubmission(u, hdr.Filename, data, exchange.backtestParams())
			if err != nil { errResp(w, err.Error(), 400); return }
			jsonResp(w, publicSubmission(s, u.IsAdmin), 200)
		default:
			errResp(w, "method not allowed", 405)
		}
	})

	// GET /api/submissions/<id>: one submission with daily results and the bot's log.
	mux.HandleFunc("/api/submissions/", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		u := auth(w, r)
		if u == nil { return }
		id := strings.TrimPrefix(r.URL.Path, "/api/submissions/")
		for _, s := range loadSubmissions(true) {
			if s.ID == id && (u.IsAdmin || s.UserID == u.ID) {
				jsonResp(w, publicSubmission(s, u.IsAdmin), 200)
				return
			}
		}
		errResp(w, "submission not found", 404)
	})

	mux.HandleFunc("/api/bot-leaderboard", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		lb := botLeaderboard()
		for i := range lb { lb[i].Params.Seed = 0 }
		jsonResp(w, lb, 200)
	})

	// Re-run every team's latest bot with the current backtest settings (e.g. a new seed for the
	// final ranking).
	mux.HandleFunc("/api/admin/rerun-bots", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == "OPTIONS" { w.WriteHeader(200); return }
		if r.Method != "POST" { errResp(w, "method not allowed", 405); return }
		u := auth(w, r)
		if u == nil { return }
		if !u.IsAdmin { errResp(w, "admin required", 400); return }

		latest := make(map[string]*Submission)
		for _, s := range loadSubmissions(false) {
			if s.Status == "done" || s.Status == "error" { latest[s.UserID] = s }
		}
		params := exchange.backtestParams()
		queued := 0
		for _, s := range latest {
			data, err := os.ReadFile(filepath.Join(botsDir, s.ID, s.File))
			if err != nil { continue }
			owner := &User{ID: s.UserID, Username: s.Username}
			if _, err := createSubmission(owner, s.Filename, data, params); err == nil { queued++ }
			time.Sleep(2 * time.Millisecond) // distinct, ordered ids
		}
		jsonResp(w, map[string]int{"queued": queued}, 200)
	})
}
