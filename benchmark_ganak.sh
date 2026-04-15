#!/usr/bin/env bash
# benchmark_ganak.sh
# ─────────────────────────────────────────────────────────────────────────────
# Baseline benchmark: runs ganak directly on every .cnf file in the input
# folder — no cube splitting, no master, no workers. Produces a CSV row for
# each file that is directly comparable with the HydraSAT benchmark CSV
# (same benchmark_label column convention: "baseline_ganak").
#
# Usage:
#   ./benchmark_ganak.sh [cnf_folder] [output_csv] [timeout_sec]
#
# Defaults:
#   cnf_folder   = cnf_instances
#   output_csv   = results/benchmark.csv   (appended, not overwritten)
#   timeout_sec  = 3600  (1 hour; 0 = no timeout)
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

CNF_FOLDER="${1:-cnf_instances}"
OUTPUT_CSV="${2:-results/benchmark.csv}"
TIMEOUT_SEC="${3:-3600}"

# ── Locate ganak ──────────────────────────────────────────────────────────────
GANAK=""
for candidate in \
    "$(command -v ganak 2>/dev/null)" \
    "/usr/local/bin/ganak" \
    "src/external/ganak-linux-amd64/ganak"; do
    if [[ -x "$candidate" ]]; then
        GANAK="$candidate"
        break
    fi
done

if [[ -z "$GANAK" ]]; then
    echo "[ganak-bench] ERROR: ganak binary not found"
    echo "              Put it in PATH or at src/external/ganak-linux-amd64/ganak"
    exit 1
fi

echo "[ganak-bench] Using ganak: $GANAK"
echo "[ganak-bench] Timeout: ${TIMEOUT_SEC}s (0=none)"

# ── Parse ganak output ────────────────────────────────────────────────────────
# Adjust the grep pattern to match your ganak build's output format.
parse_count() {
    local out="$1"
    # ganak typically prints "s SATISFIABLE" and "c model count X"
    # or "Counting... X models"  — adapt as needed.
    echo "$out" | grep -oP '(?<=s mc )\d+' | tail -1 || \
    echo "$out" | grep -oP '(?<=model count )\d+' | tail -1 || \
    echo "?"
}

mkdir -p "$(dirname "$OUTPUT_CSV")"

# Write header if the file is new
if [[ ! -f "$OUTPUT_CSV" ]]; then
    echo "timestamp,benchmark_label,worker_limit,cnf_file,model_count,completed_tasks,timed_out_tasks,total_tasks,wall_time_sec,avg_task_sec,final_timeout_sec,workers" \
        > "$OUTPUT_CSV"
fi

CNF_FILES=( "$CNF_FOLDER"/*.cnf )
TOTAL=${#CNF_FILES[@]}
echo "[ganak-bench] Found $TOTAL .cnf files in $CNF_FOLDER"
echo ""

for i in "${!CNF_FILES[@]}"; do
    CNF="${CNF_FILES[$i]}"
    NUM=$(( i + 1 ))
    echo "[ganak-bench] [$NUM/$TOTAL] $(basename "$CNF")"

    TIMED_OUT=0
    START_NS=$(date +%s%N)

    if [[ "$TIMEOUT_SEC" -gt 0 ]]; then
        OUTPUT=$(timeout "$TIMEOUT_SEC" "$GANAK" "$CNF" 2>&1) || {
            CODE=$?
            if [[ $CODE -eq 124 ]]; then   # timeout exit code
                TIMED_OUT=1
                echo "              → timed out after ${TIMEOUT_SEC}s"
            fi
            OUTPUT=""
        }
    else
        OUTPUT=$("$GANAK" "$CNF" 2>&1)
    fi

    END_NS=$(date +%s%N)
    WALL_MS=$(( (END_NS - START_NS) / 1000000 ))
    WALL_SEC=$(echo "scale=3; $WALL_MS / 1000" | bc)

    if [[ $TIMED_OUT -eq 1 ]]; then
        COUNT="?"
    else
        COUNT=$(parse_count "$OUTPUT")
    fi

    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "$TIMESTAMP,baseline_ganak,1,$CNF,$COUNT,1,$TIMED_OUT,1,$WALL_SEC,,$TIMEOUT_SEC,1" \
        >> "$OUTPUT_CSV"

    printf "              count=%-20s  time=%ss\n" "$COUNT" "$WALL_SEC"
done

echo ""
echo "[ganak-bench] Done. Results appended to $OUTPUT_CSV"
echo "[ganak-bench] Run: python3 analyze_benchmark.py $OUTPUT_CSV"
