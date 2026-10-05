package main

// Runner: backtests queued bot submissions one at a time (see submissions.go for the folder
// layout). Runs in its own container with no network, as root, and starts each bot as an
// unprivileged uid inside a private work folder, so a bot can't read other teams' uploads,
// the exchange state, or reach anything over the network.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	maxUnzipBytes = 50 << 20
	maxUnzipFiles = 500
)

type runnerConfig struct {
	jobs, work, sdk, python string
	uid, owner, memMB       int
	maxCalls                int
	timeout, tickTimeout    time.Duration
}

func runRunnerCmd(args []string) {
	fs := flag.NewFlagSet("runner", flag.ExitOnError)
	c := runnerConfig{}
	fs.StringVar(&c.jobs, "jobs", "/jobs", "shared submissions folder")
	fs.StringVar(&c.work, "work", "/work", "scratch folder for running bots")
	fs.StringVar(&c.sdk, "sdk", "/sdk", "folder with the official exchange.py")
	fs.StringVar(&c.python, "python", "python3", "python interpreter")
	fs.IntVar(&c.uid, "uid", 65534, "uid/gid to run bots as")
	fs.IntVar(&c.owner, "owner", 1001, "uid/gid of the exchange server, which writes to -jobs (0 = leave)")
	fs.IntVar(&c.memMB, "mem-mb", 1024, "address space limit per bot in MB")
	fs.IntVar(&c.maxCalls, "max-calls", 100, "API calls allowed per tick")
	fs.DurationVar(&c.timeout, "timeout", 30*time.Minute, "wall clock limit per backtest")
	fs.DurationVar(&c.tickTimeout, "tick-timeout", 5*time.Second, "wall clock limit per on_tick")
	fs.Parse(args)

	if c.owner > 0 {
		os.MkdirAll(c.jobs, 0700)
		os.Chown(c.jobs, c.owner, c.owner) // a fresh volume starts out owned by root
		os.Chmod(c.jobs, 0700)
	}
	os.MkdirAll(c.work, 0711)
	os.Chmod(c.work, 0711) // bots can enter their own folder but not list or create in /work
	log.Printf("runner: watching %s", c.jobs)
	for {
		if id := nextJob(c.jobs); id != "" {
			runJob(c, id)
			continue
		}
		time.Sleep(2 * time.Second)
	}
}

// nextJob is the oldest submission without a result. One that was running when the runner
// stopped is simply run again.
func nextJob(jobs string) string {
	entries, _ := os.ReadDir(jobs)
	for _, e := range entries { // ReadDir sorts by name; ids start with the upload time
		dir := filepath.Join(jobs, e.Name())
		if !e.IsDir() || !submissionID.MatchString(e.Name()) { continue }
		if _, err := os.Stat(filepath.Join(dir, "job.json")); err != nil { continue }
		if _, err := os.Stat(filepath.Join(dir, "result.json")); err == nil { continue }
		return e.Name()
	}
	return ""
}

// writeOwned writes a file in a job folder, owned like the folder (the exchange server's user).
func writeOwned(dir, name string, v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil { log.Printf("runner: %v", err); return }
	if st, err := os.Stat(dir); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok { os.Chown(tmp, int(sys.Uid), int(sys.Gid)) }
	}
	os.Rename(tmp, path)
}

func runJob(c runnerConfig, id string) {
	dir := filepath.Join(c.jobs, id)
	var job SubmissionJob
	if !readJSON(filepath.Join(dir, "job.json"), &job) {
		writeOwned(dir, "result.json", &BacktestResult{Status: "error", Error: "bad job.json"})
		return
	}
	log.Printf("runner: %s (%s, %s) %d days x %d min", id, job.Username, job.Filename, job.Params.Days, job.Params.DayMin)
	writeOwned(dir, "status.json", map[string]int64{"startedAt": time.Now().UnixMilli()})

	res := runSandboxed(c, dir, &job)
	res.Params = job.Params
	writeOwned(dir, "result.json", res)
	log.Printf("runner: %s %s score %.2f (%.0fs) %s", id, res.Status, res.Score, res.WallSec, res.Error)
}

func runSandboxed(c runnerConfig, dir string, job *SubmissionJob) *BacktestResult {
	fail := func(format string, a ...interface{}) *BacktestResult {
		return &BacktestResult{Status: "error", Error: fmt.Sprintf(format, a...), Days: []BacktestDay{}}
	}

	work := filepath.Join(c.work, job.ID)
	os.RemoveAll(work)
	defer os.RemoveAll(work)
	defer killUID(c.uid)
	if err := os.MkdirAll(work, 0700); err != nil { return fail("work folder: %v", err) }

	src := filepath.Join(dir, job.File)
	var botFile string
	if strings.HasSuffix(job.File, ".zip") {
		root, err := unzipBot(src, work)
		if err != nil { return fail("%v", err) }
		botFile = filepath.Join(root, "bot.py")
	} else {
		data, err := os.ReadFile(src)
		if err != nil { return fail("read upload: %v", err) }
		botFile = filepath.Join(work, "bot.py")
		if err := os.WriteFile(botFile, data, 0600); err != nil { return fail("%v", err) }
	}
	// The official SDK always wins over any exchange.py in the upload.
	if sdk, err := os.ReadFile(filepath.Join(c.sdk, "exchange.py")); err == nil {
		os.WriteFile(filepath.Join(filepath.Dir(botFile), "exchange.py"), sdk, 0600)
	}
	filepath.Walk(work, func(p string, _ os.FileInfo, err error) error {
		if err == nil { os.Lchown(p, c.uid, c.uid) }
		return nil
	})

	self, _ := os.Executable()
	p := job.Params
	cmd := exec.Command(self, "backtest", "-bot", botFile, "-sdk", c.sdk, "-python", c.python,
		"-days", strconv.Itoa(p.Days), "-day-min", strconv.Itoa(p.DayMin),
		"-seed", strconv.FormatUint(p.Seed, 10), "-warmup", strconv.Itoa(p.Warmup),
		"-uid", strconv.Itoa(c.uid), "-mem-mb", strconv.Itoa(c.memMB),
		"-max-calls", strconv.Itoa(c.maxCalls),
		"-timeout", c.timeout.String(), "-tick-timeout", c.tickTimeout.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var res BacktestResult
	if json.Unmarshal(stdout.Bytes(), &res) != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2000 { msg = msg[len(msg)-2000:] }
		return fail("backtest crashed (%v): %s", err, msg)
	}
	return &res
}

// unzipBot extracts a zip into work and returns the folder that holds bot.py (the zip root, or
// its only top-level folder).
func unzipBot(src, work string) (string, error) {
	zr, err := zip.OpenReader(src)
	if err != nil { return "", fmt.Errorf("not a valid zip: %v", err) }
	defer zr.Close()
	if len(zr.File) > maxUnzipFiles { return "", fmt.Errorf("zip has more than %d files", maxUnzipFiles) }
	var total int64
	for _, f := range zr.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("bad path in zip: %s", f.Name)
		}
		dst := filepath.Join(work, name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(dst, 0700)
			continue
		}
		if !f.Mode().IsRegular() { continue } // no symlinks
		os.MkdirAll(filepath.Dir(dst), 0700)
		rc, err := f.Open()
		if err != nil { return "", err }
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil { rc.Close(); return "", err }
		n, err := io.Copy(out, io.LimitReader(rc, maxUnzipBytes-total+1))
		rc.Close()
		out.Close()
		total += n
		if err != nil { return "", err }
		if total > maxUnzipBytes { return "", fmt.Errorf("zip unpacks to more than %d MB", maxUnzipBytes>>20) }
	}
	if _, err := os.Stat(filepath.Join(work, "bot.py")); err == nil { return work, nil }
	entries, _ := os.ReadDir(work)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "__MACOSX" { dirs = append(dirs, e.Name()) }
	}
	if len(dirs) == 1 {
		root := filepath.Join(work, dirs[0])
		if _, err := os.Stat(filepath.Join(root, "bot.py")); err == nil { return root, nil }
	}
	return "", fmt.Errorf("the zip needs a bot.py at the top level")
}

// killUID kills every process left running as uid (a bot that forked and detached).
func killUID(uid int) {
	if uid <= 0 { return }
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil { continue }
		status, err := os.ReadFile(filepath.Join("/proc", p.Name(), "status"))
		if err != nil { continue }
		for _, line := range strings.Split(string(status), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && f[0] == "Uid:" && f[1] == strconv.Itoa(uid) { syscall.Kill(pid, syscall.SIGKILL) }
		}
	}
}
