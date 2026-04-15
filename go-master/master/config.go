package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// HeuristicConfig is the in-memory representation of heuristic.json.
// Fields missing from the JSON file keep their default values.
//
// Two fields can also be overridden at runtime via environment variables,
// which is how benchmark.sh drives successive trials without editing JSON:
//
//	BENCHMARK_LABEL=4w   MAX_WORKERS=4   ./bin/master_bin
type HeuristicConfig struct {
	// ── Timeout ──────────────────────────────────────────────────────────────
	// DynamicTimeout enables adaptive per-task timeouts.
	//   false → every task gets InitialTimeoutSec (0 = no timeout).
	//   true  → first task runs with no timeout; after the first completion
	//            timeout = rollingAvg * TimeoutMultiplier.
	DynamicTimeout    bool    `json:"dynamicTimeout"`
	TimeoutMultiplier float64 `json:"timeoutMultiplier"`
	InitialTimeoutSec int32   `json:"initialTimeoutSec"`

	// ── Input ─────────────────────────────────────────────────────────────────
	ReadFolder  bool   `json:"readFolder"`
	InputFolder string `json:"inputFolder"`

	// ── Output ────────────────────────────────────────────────────────────────
	OutputLog string `json:"outputLog"`

	// ── Benchmarking ─────────────────────────────────────────────────────────
	// MaxWorkers limits how many workers may hold an active task simultaneously.
	// 0 means no limit (use every connected worker).
	// This lets you connect all workers once and vary the active count per trial
	// by just restarting the master — no worker-side changes needed.
	MaxWorkers int `json:"maxWorkers"`

	// BenchmarkLabel is a short tag written into every CSV row for this run,
	// e.g. "4w_trial1". Override with the BENCHMARK_LABEL env var so benchmark.sh
	// can drive multiple trials without touching the JSON file.
	BenchmarkLabel string `json:"benchmarkLabel"`
}

func defaultConfig() HeuristicConfig {
	return HeuristicConfig{
		DynamicTimeout:    false,
		TimeoutMultiplier: 2.0,
		InitialTimeoutSec: 30,
		ReadFolder:        false,
		InputFolder:       "cnf_instances",
		OutputLog:         "results/benchmark.csv",
		MaxWorkers:        0,
		BenchmarkLabel:    "",
	}
}

// LoadHeuristicConfig reads path and merges it on top of defaults.
// If the file does not exist, defaults are returned without error.
// After loading, environment variables MAX_WORKERS and BENCHMARK_LABEL
// override their JSON counterparts if set.
func LoadHeuristicConfig(path string) (*HeuristicConfig, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Printf("[config] %s not found — using defaults\n", path)
	} else if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}

	// ── Environment variable overrides ────────────────────────────────────────
	// These let benchmark.sh drive successive trials without editing JSON.
	if v := os.Getenv("BENCHMARK_LABEL"); v != "" {
		cfg.BenchmarkLabel = v
	}
	if v := os.Getenv("MAX_WORKERS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			cfg.MaxWorkers = n
		}
	}

	printConfig(&cfg)
	return &cfg, nil
}

func printConfig(cfg *HeuristicConfig) {
	fmt.Println("[config] ──────────────────────────────────────")
	fmt.Printf("[config]  dynamicTimeout    : %v\n", cfg.DynamicTimeout)
	if cfg.DynamicTimeout {
		fmt.Printf("[config]  timeoutMultiplier : %.2f×\n", cfg.TimeoutMultiplier)
		fmt.Printf("[config]  initialTimeoutSec : %d (0=∞ until first completion)\n", cfg.InitialTimeoutSec)
	} else {
		fmt.Printf("[config]  initialTimeoutSec : %d\n", cfg.InitialTimeoutSec)
	}
	fmt.Printf("[config]  readFolder        : %v\n", cfg.ReadFolder)
	if cfg.ReadFolder {
		fmt.Printf("[config]  inputFolder       : %s\n", cfg.InputFolder)
	}
	fmt.Printf("[config]  outputLog         : %s\n", cfg.OutputLog)
	fmt.Printf("[config]  maxWorkers        : %d (0=unlimited)\n", cfg.MaxWorkers)
	fmt.Printf("[config]  benchmarkLabel    : %q\n", cfg.BenchmarkLabel)
	fmt.Println("[config] ──────────────────────────────────────")
}
