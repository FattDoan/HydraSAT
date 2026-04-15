#!/usr/bin/env bash
# benchmark.sh
# ─────────────────────────────────────────────────────────────────────────────
# Runs the full benchmark suite across all CNF instances for each worker-count
# tier. Workers on both VMs stay connected throughout — the master limits
# concurrency via MAX_WORKERS. You only need to run this script once on the
# master machine.
#
# Usage:
#   ./benchmark.sh [--tiers "1 4 8 16"] [--trials 1] [--port 1208]
#
# Prerequisites:
#   - bin/master_bin is built  (make master)
#   - Workers are already running on all VMs (make noroot-worker-up CORES=N)
#   - heuristic.json exists with readFolder:true and inputFolder set
#   - results/ directory will be created automatically
#
# Output:
#   results/benchmark.csv   — one row per (trial, cnf_file) combination
#   results/benchmark.log   — stdout+stderr of every master run
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

MASTER_BIN="${MASTER_BIN:-./bin/master_bin}"
PORT="${PORT:-1208}"
TIERS="${TIERS:-1 4 8 16}"
TRIALS="${TRIALS:-1}"           # repeat each tier this many times (for variance)
RESULTS_DIR="${RESULTS_DIR:-results}"
LOG_FILE="${RESULTS_DIR}/benchmark.log"
WARMUP_SEC="${WARMUP_SEC:-5}"   # seconds to wait for workers to (re)connect

# ── Sanity checks ─────────────────────────────────────────────────────────────
if [[ ! -f "$MASTER_BIN" ]]; then
    echo "[bench] ERROR: master binary not found at $MASTER_BIN"
    echo "        Run: make master"
    exit 1
fi

if [[ ! -f "heuristic.json" ]]; then
    echo "[bench] ERROR: heuristic.json not found"
    echo "        Run: make heuristic  (creates a default one)"
    exit 1
fi

INPUT_FOLDER=$(python3 -c "import json,sys; d=json.load(open('heuristic.json')); print(d.get('inputFolder','cnf_instances'))" 2>/dev/null || echo "cnf_instances")
if [[ ! -d "$INPUT_FOLDER" ]]; then
    echo "[bench] ERROR: inputFolder '$INPUT_FOLDER' does not exist"
    exit 1
fi

CNF_COUNT=$(find "$INPUT_FOLDER" -maxdepth 1 -name "*.cnf" | wc -l)
if [[ "$CNF_COUNT" -eq 0 ]]; then
    echo "[bench] ERROR: no .cnf files found in $INPUT_FOLDER"
    exit 1
fi

mkdir -p "$RESULTS_DIR"

# ── Summary header ────────────────────────────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║              HydraSAT Benchmark Suite                       ║"
echo "╠══════════════════════════════════════════════════════════════╣"
printf "║  Instances   : %-3d .cnf files in %-21s     ║\n" "$CNF_COUNT" "$INPUT_FOLDER"
printf "║  Worker tiers: %-44s ║\n" "$TIERS"
printf "║  Trials/tier : %-44s ║\n" "$TRIALS"
printf "║  Output CSV  : %-44s ║\n" "results/benchmark.csv"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

TOTAL_RUNS=$(( $(echo $TIERS | wc -w) * TRIALS ))
RUN_NUM=0
SUITE_START=$(date +%s)

# ── Main loop ─────────────────────────────────────────────────────────────────
for WORKERS in $TIERS; do
    for TRIAL in $(seq 1 "$TRIALS"); do
        RUN_NUM=$(( RUN_NUM + 1 ))
        LABEL="${WORKERS}w_trial${TRIAL}"

        echo "────────────────────────────────────────────────────────────────"
        echo "[bench] Run $RUN_NUM/$TOTAL_RUNS  label=$LABEL  maxWorkers=$WORKERS"
        echo "[bench] Waiting ${WARMUP_SEC}s for workers to (re)connect…"
        sleep "$WARMUP_SEC"

        RUN_START=$(date +%s)

        # Drive the master entirely via env vars — no JSON editing needed.
        BENCHMARK_LABEL="$LABEL" \
        MAX_WORKERS="$WORKERS" \
        PORT="$PORT" \
            "$MASTER_BIN" 2>&1 | tee -a "$LOG_FILE"

        RUN_END=$(date +%s)
        ELAPSED=$(( RUN_END - RUN_START ))
        echo "[bench] ✓ label=$LABEL finished in ${ELAPSED}s"
        echo ""
    done
done

SUITE_END=$(date +%s)
SUITE_ELAPSED=$(( SUITE_END - SUITE_START ))
echo "════════════════════════════════════════════════════════════════"
echo "[bench] All $TOTAL_RUNS runs complete in ${SUITE_ELAPSED}s"
echo "[bench] Results: $RESULTS_DIR/benchmark.csv"
echo ""
echo "[bench] To generate analysis:"
echo "        python3 analyze_benchmark.py $RESULTS_DIR/benchmark.csv"
echo ""
