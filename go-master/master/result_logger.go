package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ResultLogger appends one CSV row per solved problem.
// Thread-safe; multiple goroutines may call LogResult concurrently.
type ResultLogger struct {
	path string
	mu   sync.Mutex
}

// csvHeader defines every column in order. Keep in sync with LogResult's
// w.Write call and with analyze_benchmark.py's column references.
var csvHeader = []string{
	"timestamp",
	"benchmark_label",   // e.g. "4w_trial1" — set via heuristic.json or BENCHMARK_LABEL env
	"worker_limit",      // maxWorkers config value (0 = unlimited)
	"cnf_file",
	"model_count",
	"completed_tasks",
	"timed_out_tasks",   // tasks that hit the timeout and were split
	"total_tasks",       // completed + timed_out + any still in flight (should be 0 at log time)
	"wall_time_sec",
	"avg_task_sec",
	"final_timeout_sec", // "none" when dynamic timeout had not yet engaged
	"workers",           // total registered workers (not just active)
}

// NewResultLogger creates (or opens) the CSV at path and writes the header
// row if the file is new. The directory is created automatically.
func NewResultLogger(path string) (*ResultLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	isNew := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		isNew = true
	}

	if isNew {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("creating log file: %w", err)
		}
		w := csv.NewWriter(f)
		_ = w.Write(csvHeader)
		w.Flush()
		f.Close()
	}

	return &ResultLogger{path: path}, nil
}

// LogResult appends a single result row to the CSV.
func (rl *ResultLogger) LogResult(
	benchmarkLabel  string,
	workerLimit     int,
	cnfFile         string,
	modelCount      string,
	completedTasks  int32,
	timedOutTasks   int32,
	totalTasks      int32,
	wallTimeSec     float64,
	avgTaskSec      float64,
	finalTimeoutSec int32,
	workerCount     int32,
) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	f, err := os.OpenFile(rl.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("[logger] failed to open log: %v\n", err)
		return
	}
	defer f.Close()

	timeoutStr := fmt.Sprintf("%d", finalTimeoutSec)
	if finalTimeoutSec == 2147483647 { // math.MaxInt32 sentinel = no timeout
		timeoutStr = "none"
	}

	w := csv.NewWriter(f)
	_ = w.Write([]string{
		time.Now().UTC().Format(time.RFC3339),
		benchmarkLabel,
		fmt.Sprintf("%d", workerLimit),
		cnfFile,
		modelCount,
		fmt.Sprintf("%d", completedTasks),
		fmt.Sprintf("%d", timedOutTasks),
		fmt.Sprintf("%d", totalTasks),
		fmt.Sprintf("%.3f", wallTimeSec),
		fmt.Sprintf("%.3f", avgTaskSec),
		timeoutStr,
		fmt.Sprintf("%d", workerCount),
	})
	w.Flush()

	fmt.Printf("[logger] %-30s  label=%-12s  workers=%d  count=%s  time=%.1fs\n",
		filepath.Base(cnfFile), benchmarkLabel, workerCount, modelCount, wallTimeSec)
}
