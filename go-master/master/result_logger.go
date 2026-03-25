package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ResultLogger appends one CSV row per solved problem to a log file.
// It is safe for concurrent use.
type ResultLogger struct {
	path string
	mu   sync.Mutex
}

// NewResultLogger creates (or opens) the CSV at path and writes a header
// row if the file is new.
func NewResultLogger(path string) (*ResultLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	isNew := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		isNew = true
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	if isNew {
		w := csv.NewWriter(f)
		_ = w.Write([]string{
			"timestamp",
			"cnf_file",
			"model_count",
			"completed_tasks",
			"total_tasks",
			"wall_time_sec",
			"avg_task_sec",
			"final_timeout_sec",
			"workers",
		})
		w.Flush()
	}

	return &ResultLogger{path: path}, nil
}

// LogResult appends a result row. All fields are pre-formatted strings so
// callers don't need to import this package's types.
func (rl *ResultLogger) LogResult(
	cnfFile string,
	modelCount string,
	completedTasks int32,
	totalTasks int32,
	wallTimeSec float64,
	avgTaskSec float64,
	finalTimeoutSec int32,
	workerCount int32,
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
	if finalTimeoutSec == 2147483647 { // math.MaxInt32 sentinel
		timeoutStr = "none"
	}

	w := csv.NewWriter(f)
	_ = w.Write([]string{
		time.Now().UTC().Format(time.RFC3339),
		cnfFile,
		modelCount,
		fmt.Sprintf("%d", completedTasks),
		fmt.Sprintf("%d", totalTasks),
		fmt.Sprintf("%.3f", wallTimeSec),
		fmt.Sprintf("%.3f", avgTaskSec),
		timeoutStr,
		fmt.Sprintf("%d", workerCount),
	})
	w.Flush()

	fmt.Printf("[logger] result written to %s\n", rl.path)
}
