package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func RenderUI(stats *StatsCollector, masterAddr string, width, height int) string {
	s := stats.GetStats()
	var sb strings.Builder
	sb.WriteString(renderHeader(masterAddr, width))
	sb.WriteString("\n")
	sb.WriteString(renderSystemOverview(s, width))
	sb.WriteString("\n\n")
	sb.WriteString(renderWorkerTable(s, width, height-15))
	sb.WriteString("\n")
	sb.WriteString(renderFooter(width))
	return sb.String()
}

func renderHeader(masterAddr string, width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FF00")).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(0, 1).
		Width(width)
	timestamp := time.Now().Format("15:04:05")
	return titleStyle.Render(fmt.Sprintf("HydraSAT Monitor - %s - Master: %s", timestamp, masterAddr))
}

func renderSystemOverview(s SystemStats, width int) string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3498db")).
		Padding(0, 1)

	totalCPU, totalRAM := 0.0, 0.0
	for _, w := range s.Workers {
		totalCPU += w.CPUUsage
		totalRAM += w.MemoryUsageMB
	}
	avgCPU := 0.0
	if len(s.Workers) > 0 {
		avgCPU = totalCPU / float64(len(s.Workers))
	}
	cpuBar := renderProgressBar(avgCPU, 30, "#e74c3c", "#2ecc71")

	avgTaskStr := "n/a (no completions yet)"
	if s.AvgTaskSec > 0 {
		avgTaskStr = fmt.Sprintf("%.2fs", s.AvgTaskSec)
	}

	timeoutStr := "none"
	if s.CurrentTimeoutSec > 0 {
		timeoutStr = fmt.Sprintf("%.0fs  (= avg × %.1fx)",
			s.CurrentTimeoutSec, s.CurrentTimeoutSec/max(s.AvgTaskSec, 0.001))
	}

	dynamicLabel := "static timeout"
	dynamicColor := "#95a5a6"
	if s.DynamicTimeout {
		dynamicLabel = "dynamic timeout ✓"
		dynamicColor = "#2ecc71"
	}
	modeStr := lipgloss.NewStyle().Foreground(lipgloss.Color(dynamicColor)).Render(dynamicLabel)

	overview := fmt.Sprintf(
		"Workers: %d/%d busy | Avg CPU: %s %.1f%% | Total RAM: %.0f MB\n"+
			"Active: %d | Completed: %d | Queued: %d | Count: %s\n"+
			"Avg task: %s | Timeout: %s | Mode: %s",
		s.BusyWorkers, s.TotalWorkers,
		cpuBar, avgCPU, totalRAM,
		s.ActiveTasks, s.CompletedTasks, s.QueuedTasks, s.TotalCount,
		avgTaskStr, timeoutStr, modeStr,
	)
	return boxStyle.Render(overview)
}

func renderWorkerTable(s SystemStats, width, height int) string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#f39c12")).
		Background(lipgloss.Color("#2c3e50")).
		Padding(0, 1)

	hostnameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#3498db")).
		Background(lipgloss.Color("#34495e")).
		Padding(0, 1)

	busyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#2ecc71")).Padding(0, 1)
	idleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#95a5a6")).Padding(0, 1)
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e74c3c")).Padding(0, 1)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f39c12")).Padding(0, 1)

	var sb strings.Builder

	// ELAPSED and REMAINING are the two new debug columns
	header := fmt.Sprintf(
		"%-18s %-9s %-7s %-10s %-10s %-14s %-14s %s",
		"WORKER ID", "STATUS", "TASK", "ELAPSED", "REMAINING", "TIMEOUT", "CPU / RAM", "CUBE",
	)
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\n")

	// Group by hostname
	groups := make(map[string][]*WorkerInfo)
	for _, w := range s.Workers {
		h := w.Hostname
		if h == "" {
			h = "unknown"
		}
		groups[h] = append(groups[h], w)
	}
	hostnames := make([]string, 0, len(groups))
	for h := range groups {
		hostnames = append(hostnames, h)
	}
	sort.Strings(hostnames)

	for _, hostname := range hostnames {
		workers := groups[hostname]
		sort.Slice(workers, func(i, j int) bool { return workers[i].ID < workers[j].ID })

		sb.WriteString(hostnameStyle.Render(fmt.Sprintf("📍 %s (%d workers)", hostname, len(workers))))
		sb.WriteString("\n")

		for _, worker := range workers {
			var rowStyle lipgloss.Style
			switch worker.Status {
			case "BUSY":
				rowStyle = busyStyle
			case "IDLE":
				rowStyle = idleStyle
			case "ERROR", "TIMEOUT":
				rowStyle = errorStyle
			default:
				rowStyle = lipgloss.NewStyle().Padding(0, 1)
			}

			taskID := "-"
			if worker.CurrentTaskID != -1 {
				taskID = fmt.Sprintf("#%d", worker.CurrentTaskID)
			}

			elapsedStr := "-"
			remainingStr := "-"
			timeoutColStr := "-"

			if worker.Status == "BUSY" {
				elapsedStr = fmt.Sprintf("%.1fs", worker.TaskElapsedSec)

				if worker.TaskTimeout <= 0 || worker.TaskTimeout >= 2_147_483_647 {
					// No timeout assigned
					timeoutColStr = "∞"
					remainingStr = "∞"
				} else {
					timeoutColStr = fmt.Sprintf("%.0fs", worker.TaskTimeout)
					remaining := worker.TaskTimeout - worker.TaskElapsedSec

					if remaining <= 0 {
						// Overdue — should have been killed already
						remainingStr = errorStyle.Render("OVERDUE")
					} else {
						pct := remaining / worker.TaskTimeout
						switch {
						case pct < 0.15:
							remainingStr = errorStyle.Render(fmt.Sprintf("%.1fs!", remaining))
						case pct < 0.35:
							remainingStr = warnStyle.Render(fmt.Sprintf("%.1fs", remaining))
						default:
							remainingStr = fmt.Sprintf("%.1fs", remaining)
						}
					}
				}
			}

			cpuRAM := fmt.Sprintf("%5.1f%% %4.0fM", worker.CPUUsage, worker.MemoryUsageMB)
			cube := formatCubeFull(worker.CurrentCube, width-95)

			row := fmt.Sprintf(
				"  %-16s %-9s %-7s %-10s %-10s %-14s %-14s %s",
				truncate(worker.ID, 16),
				worker.Status,
				taskID,
				elapsedStr,
				remainingStr,
				timeoutColStr,
				cpuRAM,
				cube,
			)
			sb.WriteString(rowStyle.Render(row))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(s.Workers) == 0 {
		sb.WriteString(idleStyle.Render("No workers connected yet..."))
		sb.WriteString("\n")
	}
	return sb.String()
}

func renderFooter(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7f8c8d")).
		Italic(true).
		Padding(0, 1).
		Render("[q/Ctrl+C/Esc] Quit  [r] Refresh")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func renderProgressBar(percent, width float64, colorHigh, colorLow string) string {
	filled := int(percent / 100 * width)
	if filled > int(width) {
		filled = int(width)
	}
	if filled < 0 {
		filled = 0
	}
	color := colorLow
	if percent > 80 {
		color = colorHigh
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", int(width)-filled)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(bar)
}

func formatCubeFull(cube []int32, maxWidth int) string {
	if len(cube) == 0 {
		return "-"
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, lit := range cube {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%d", lit))
		if sb.Len() > maxWidth && i < len(cube)-1 {
			sb.WriteString(fmt.Sprintf(", ...] (%d total)", len(cube)))
			return sb.String()
		}
	}
	sb.WriteString("]")
	return sb.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
