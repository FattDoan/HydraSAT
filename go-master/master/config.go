package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// HeuristicConfig is the canonical representation of heuristic.json.
// All master subsystems receive a pointer to this struct so new knobs
// only need to be added in one place.
type HeuristicConfig struct {
	// DynamicTimeout: when true the per-task timeout is derived from the
	// running average of completed task durations times TimeoutMultiplier.
	// When false (or before any task finishes) InitialTimeoutSec is used.
	// InitialTimeoutSec == 0 means "no timeout" until the average is known.
	DynamicTimeout    bool    `json:"dynamicTimeout"`
	InitialTimeoutSec float64 `json:"initialTimeoutSec"`
	TimeoutMultiplier float64 `json:"timeoutMultiplier"`

	// ReadFolder: when true the master scans InputFolder for *.cnf files
	// and enqueues them all as separate solve jobs instead of reading a
	// single file from the CLI argument.
	ReadFolder  bool   `json:"readFolder"`
	InputFolder string `json:"inputFolder"`

	// OutputLog is the path of the CSV file where per-problem results are
	// appended when a solve finishes.
	OutputLog string `json:"outputLog"`
}

// DefaultHeuristicConfig returns safe defaults so the master works even
// without a heuristic.json file (mirrors the old hard-coded behaviour).
func DefaultHeuristicConfig() *HeuristicConfig {
	return &HeuristicConfig{
		DynamicTimeout:    false,
		InitialTimeoutSec: 30,
		TimeoutMultiplier: 2.0,
		ReadFolder:        false,
		InputFolder:       "./cnf-input",
		OutputLog:         "./results/hydrasat_results.csv",
	}
}

// LoadHeuristicConfig reads heuristic.json from the given path.
// Missing file → defaults; parse error → error.
func LoadHeuristicConfig(path string) (*HeuristicConfig, error) {
	cfg := DefaultHeuristicConfig()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Printf("[config] %s not found — using defaults\n", path)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Sanity-check multiplier
	if cfg.TimeoutMultiplier <= 0 {
		cfg.TimeoutMultiplier = 2.0
	}

	fmt.Printf("[config] Loaded %s\n", path)
	fmt.Printf("  dynamicTimeout   = %v  (initial=%.0fs  multiplier=%.1fx)\n",
		cfg.DynamicTimeout, cfg.InitialTimeoutSec, cfg.TimeoutMultiplier)
	fmt.Printf("  readFolder       = %v  (%s)\n", cfg.ReadFolder, cfg.InputFolder)
	fmt.Printf("  outputLog        = %s\n", cfg.OutputLog)

	return cfg, nil
}
