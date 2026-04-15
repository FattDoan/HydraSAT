#!/usr/bin/env python3
"""
analyze_benchmark.py
────────────────────────────────────────────────────────────────────────────
Reads the benchmark CSV produced by HydraSAT (result_logger.go) and
benchmark_ganak.sh, then produces:

  1. Console summary table (per label: total wall time, mean per problem)
  2. Per-problem speedup table (vs baseline_ganak)
  3. results/summary.csv   — machine-readable aggregates
  4. results/table.tex     — LaTeX booktabs table for inclusion in the report

Usage:
  python3 analyze_benchmark.py results/benchmark.csv [--baseline baseline_ganak]
"""

import argparse
import csv
import sys
from collections import defaultdict
from pathlib import Path


# ── helpers ───────────────────────────────────────────────────────────────────

def fmt_time(sec: float) -> str:
    if sec >= 3600:
        return f"{sec/3600:.2f}h"
    if sec >= 60:
        return f"{sec/60:.1f}m"
    return f"{sec:.1f}s"

def fmt_speedup(s: float) -> str:
    return f"{s:.2f}×"


# ── load CSV ──────────────────────────────────────────────────────────────────

def load(path: str) -> list[dict]:
    rows = []
    with open(path, newline="") as f:
        reader = csv.DictReader(f)
        for row in reader:
            try:
                row["wall_time_sec"] = float(row["wall_time_sec"])
            except (ValueError, KeyError):
                row["wall_time_sec"] = float("nan")
            rows.append(row)
    return rows


# ── aggregate by label ────────────────────────────────────────────────────────

def aggregate(rows: list[dict]) -> dict:
    """Returns {label: {cnf_file: wall_time_sec, ...}}"""
    data: dict[str, dict[str, float]] = defaultdict(dict)
    for row in rows:
        label = row.get("benchmark_label", "?")
        cnf   = Path(row.get("cnf_file", "?")).name
        wall  = row["wall_time_sec"]
        # If a problem appears multiple times under the same label (multiple trials),
        # keep the minimum (best-case) — change to mean() if you prefer average.
        if cnf not in data[label] or wall < data[label][cnf]:
            data[label][cnf] = wall
    return data


# ── print console table ───────────────────────────────────────────────────────

def print_summary(data: dict, baseline: str) -> None:
    labels = sorted(data.keys(), key=lambda l: (l != baseline, l))
    all_cnfs = sorted({cnf for by_cnf in data.values() for cnf in by_cnf})

    # Per-label totals
    print()
    print("┌" + "─"*72 + "┐")
    print("│{:^72}│".format("BENCHMARK SUMMARY"))
    print("├" + "─"*28 + "┬" + "─"*12 + "┬" + "─"*12 + "┬" + "─"*16 + "┤")
    print(f"│ {'LABEL':<26} │ {'TOTAL':>10} │ {'MEAN':>10} │ {'SPEEDUP vs base':>14} │")
    print("├" + "─"*28 + "┼" + "─"*12 + "┼" + "─"*12 + "┼" + "─"*16 + "┤")

    baseline_total = sum(data.get(baseline, {}).values()) or None

    for label in labels:
        by_cnf = data[label]
        total  = sum(by_cnf.values())
        mean   = total / len(by_cnf) if by_cnf else float("nan")
        speedup_str = "—"
        if baseline_total and label != baseline:
            speedup_str = fmt_speedup(baseline_total / total)
        print(f"│ {label:<26} │ {fmt_time(total):>10} │ {fmt_time(mean):>10} │ {speedup_str:>14} │")

    print("└" + "─"*28 + "┴" + "─"*12 + "┴" + "─"*12 + "┴" + "─"*16 + "┘")
    print(f"  {len(all_cnfs)} instances")
    print()


# ── per-problem speedup table ─────────────────────────────────────────────────

def print_per_problem(data: dict, baseline: str) -> None:
    if baseline not in data:
        print(f"[analyze] baseline label '{baseline}' not found — skipping per-problem table")
        return

    labels_no_base = sorted(l for l in data if l != baseline)
    if not labels_no_base:
        return

    base_times = data[baseline]
    all_cnfs   = sorted(base_times.keys())

    print("Per-problem speedup (vs baseline_ganak):")
    header = f"  {'INSTANCE':<30}" + "".join(f"  {l:>12}" for l in labels_no_base)
    print(header)
    print("  " + "─" * (len(header) - 2))

    for cnf in all_cnfs:
        base_t = base_times.get(cnf)
        row_str = f"  {cnf:<30}"
        for label in labels_no_base:
            t = data[label].get(cnf)
            if t and base_t:
                row_str += f"  {fmt_speedup(base_t/t):>12}"
            else:
                row_str += f"  {'n/a':>12}"
        print(row_str)
    print()


# ── write summary CSV ─────────────────────────────────────────────────────────

def write_summary_csv(data: dict, baseline: str, out_path: str) -> None:
    labels    = sorted(data.keys(), key=lambda l: (l != baseline, l))
    all_cnfs  = sorted({cnf for by_cnf in data.values() for cnf in by_cnf})
    base_times = data.get(baseline, {})

    with open(out_path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["cnf_file"] + labels + [f"speedup_{l}" for l in labels if l != baseline])
        for cnf in all_cnfs:
            row = [cnf]
            for label in labels:
                t = data[label].get(cnf, "")
                row.append(f"{t:.3f}" if isinstance(t, float) else t)
            # Speedup columns
            base_t = base_times.get(cnf)
            for label in labels:
                if label == baseline:
                    continue
                t = data[label].get(cnf)
                if t and base_t:
                    row.append(f"{base_t/t:.3f}")
                else:
                    row.append("")
            w.writerow(row)

    print(f"[analyze] Summary CSV → {out_path}")


# ── write LaTeX table ─────────────────────────────────────────────────────────

def write_latex(data: dict, baseline: str, out_path: str) -> None:
    labels_no_base = sorted(l for l in data if l != baseline)
    all_cnfs = sorted({cnf for by_cnf in data.values() for cnf in by_cnf})
    base_times = data.get(baseline, {})

    # Column spec: instance | baseline | one col per non-baseline label (speedup)
    n_extra = len(labels_no_base)
    col_spec = "l" + "r" * (1 + n_extra)

    lines = [
        r"\begin{table}[t]",
        r"\centering",
        r"\small",
        rf"\begin{{tabular}}{{{col_spec}}}",
        r"\toprule",
    ]

    # Header row
    extra_hdrs = " & ".join(
        rf"\multicolumn{{1}}{{c}}{{{l.replace('_', r'\_')}}}"
        for l in labels_no_base
    )
    speedup_note = r"speedup $\uparrow$"
    lines.append(
        rf"Instance & Baseline (s) & {extra_hdrs} \\"
    )
    lines.append(
        rf" & & " + " & ".join([speedup_note] * n_extra) + r" \\"
    )
    lines.append(r"\midrule")

    for cnf in all_cnfs:
        base_t = base_times.get(cnf)
        cnf_tex = cnf.replace("_", r"\_").replace(".cnf", "")

        base_str = f"{base_t:.1f}" if base_t else "—"
        extra_cols = []
        for label in labels_no_base:
            t = data[label].get(cnf)
            if t and base_t:
                extra_cols.append(f"{base_t/t:.2f}×")
            else:
                extra_cols.append("—")

        row = f"{cnf_tex} & {base_str} & " + " & ".join(extra_cols) + r" \\"
        lines.append(row)

    lines += [
        r"\midrule",
    ]
    # Totals row
    base_total = sum(base_times.values()) if base_times else None
    total_base_str = f"{base_total:.1f}" if base_total else "—"
    total_extras = []
    for label in labels_no_base:
        label_total = sum(data[label].values())
        if base_total:
            total_extras.append(f"{base_total/label_total:.2f}×")
        else:
            total_extras.append("—")

    lines.append(
        r"\textbf{Total} & \textbf{" + total_base_str + r"} & " +
        " & ".join(rf"\textbf{{{s}}}" for s in total_extras) + r" \\"
    )
    lines += [
        r"\bottomrule",
        r"\end{tabular}",
        r"\caption{Wall-clock time (seconds) and speedup relative to sequential \texttt{ganak}"
        r" for each worker configuration. Speedup $> 1$ indicates improvement over baseline.}",
        r"\label{tab:benchmark}",
        r"\end{table}",
    ]

    Path(out_path).write_text("\n".join(lines) + "\n")
    print(f"[analyze] LaTeX table   → {out_path}")


# ── main ──────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(description="Analyze HydraSAT benchmark CSV")
    parser.add_argument("csv", help="Path to benchmark.csv")
    parser.add_argument("--baseline", default="baseline_ganak",
                        help="benchmark_label to use as speedup baseline (default: baseline_ganak)")
    args = parser.parse_args()

    rows = load(args.csv)
    if not rows:
        print("[analyze] CSV is empty")
        sys.exit(1)

    data = aggregate(rows)
    print(f"[analyze] Loaded {len(rows)} rows, {len(data)} labels, "
          f"{len({Path(r['cnf_file']).name for r in rows})} instances")

    print_summary(data, args.baseline)
    print_per_problem(data, args.baseline)

    out_dir = Path(args.csv).parent
    write_summary_csv(data, args.baseline, str(out_dir / "summary.csv"))
    write_latex(data, args.baseline, str(out_dir / "table.tex"))


if __name__ == "__main__":
    main()
