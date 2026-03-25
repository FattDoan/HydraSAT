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

	// Header
	sb.WriteString(renderHeader(masterAddr, width))
	sb.WriteString("\n")

	// System overview (aggregate worker stats)
	sb.WriteString(renderSystemOverview(s, width))
	sb.WriteString("\n\n")

	// Worker table (grouped by hostname, sorted consistently)
	sb.WriteString(renderWorkerTable(s, width, height-15))
	sb.WriteString("\n")

	// Footer
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
	title := fmt.Sprintf("🌊 HydraSAT Monitor - %s - Master: %s", timestamp, masterAddr)

	return titleStyle.Render(title)
}

func renderSystemOverview(s SystemStats, width int) string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3498db")).
		Padding(0, 1)

	// Aggregate worker CPU/RAM
	totalCPU := 0.0
	totalRAM := 0.0
	workerCount := 0
	for _, w := range s.Workers {
		totalCPU += w.CPUUsage
		totalRAM += w.MemoryUsageMB
		workerCount++
	}
	avgCPU := 0.0
	if workerCount > 0 {
		avgCPU = totalCPU / float64(workerCount)
	}

	// CPU bar (average across workers)
	cpuBar := renderProgressBar(avgCPU, 30, "#e74c3c", "#2ecc71")

	overview := fmt.Sprintf(
		"Workers: %d/%d busy | Avg CPU: %s %.1f%% | Total RAM: %.0f MB\n"+
			"Active: %d | Completed: %d | Queued: %d\n"+
			"Total Count: %s | Uptime: %s",
		s.BusyWorkers,
		s.TotalWorkers,
		cpuBar, avgCPU,
		totalRAM,
		s.ActiveTasks,
		s.CompletedTasks,
		s.QueuedTasks,
		s.TotalCount,
		formatDuration(time.Duration(s.Uptime*float64(time.Second))),
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

	busyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#2ecc71")).
		Padding(0, 1)

	idleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#95a5a6")).
		Padding(0, 1)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#e74c3c")).
		Padding(0, 1)

	var sb strings.Builder

	// Table header - REMOVED PROGRESS column, EXPANDED CUBE column
	header := fmt.Sprintf(
		"%-20s %-10s %-8s %-12s %-15s %s",
		"WORKER ID",
		"STATUS",
		"TASK",
		"ELAPSED",
		"CPU / RAM",
		"CUBE",
	)
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\n")

	// Group workers by hostname
	hostnameGroups := make(map[string][]*WorkerInfo)
	for _, worker := range s.Workers {
		hostname := worker.Hostname
		if hostname == "" {
			hostname = "unknown"
		}
		hostnameGroups[hostname] = append(hostnameGroups[hostname], worker)
	}

	// Sort hostnames alphabetically
	hostnames := make([]string, 0, len(hostnameGroups))
	for h := range hostnameGroups {
		hostnames = append(hostnames, h)
	}
	sort.Strings(hostnames)

	// Render workers grouped by hostname
	for _, hostname := range hostnames {
		workers := hostnameGroups[hostname]

		// Sort workers within hostname by ID
		sort.Slice(workers, func(i, j int) bool {
			return workers[i].ID < workers[j].ID
		})

		// Hostname header
		hostnameHeader := fmt.Sprintf("📍 %s (%d workers)", hostname, len(workers))
		sb.WriteString(hostnameStyle.Render(hostnameHeader))
		sb.WriteString("\n")

		// Worker rows
		for _, worker := range workers {
			var style lipgloss.Style
			switch worker.Status {
			case "BUSY":
				style = busyStyle
			case "IDLE":
				style = idleStyle
			case "ERROR", "TIMEOUT":
				style = errorStyle
			default:
				style = lipgloss.NewStyle().Padding(0, 1)
			}

			taskID := "-"
			if worker.CurrentTaskID != -1 {
				taskID = fmt.Sprintf("#%d", worker.CurrentTaskID)
			}

			// Simple elapsed time string instead of progress bar
			elapsedStr := "-"
			if worker.Status == "BUSY" {
				elapsedStr = fmt.Sprintf("%.1fs", worker.TaskElapsedSec)
			}

			// CPU/RAM info - formatting adjusted for neatness
			cpuRAM := fmt.Sprintf("%5.1f%% | %4.0fM", worker.CPUUsage, worker.MemoryUsageMB)

			// Full cube display
			cube := formatCubeFull(worker.CurrentCube, width-75)

			row := fmt.Sprintf(
				"  %-18s %-10s %-8s %-12s %-15s %s",
				truncate(worker.ID, 18),
				worker.Status,
				taskID,
				elapsedStr,
				cpuRAM,
				cube,
			)

			sb.WriteString(style.Render(row))
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
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7f8c8d")).
		Italic(true).
		Padding(0, 1)

	footer := "[q/Ctrl+C/Esc] Quit  [r] Refresh"
	return footerStyle.Render(footer)
}

// Helper functions

func renderProgressBar(percent float64, width int, colorHigh, colorLow string) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	color := colorLow
	if percent > 80 {
		color = colorHigh
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	return style.Render(bar)
}

func renderTimeProgress(elapsed float64, timeout float64) string {
	if elapsed == 0 || timeout == 0 {
		return "-"
	}

	// Calculate percentage
	percent := (elapsed / timeout) * 100
	if percent > 100 {
		percent = 100
	}

	// Smaller progress bar (10 chars)
	barWidth := 10
	filled := int(percent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	// Color: green -> yellow -> red
	var color string
	if percent < 50 {
		color = "#2ecc71" // Green
	} else if percent < 80 {
		color = "#f39c12" // Yellow
	} else {
		color = "#e74c3c" // Red
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))

	// Format: "12s ████░░░░░░"
	return fmt.Sprintf("%3.0fs %s", elapsed, style.Render(bar))
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}

	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// formatCubeFull shows full cube with smart truncation only if very long
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

		// Only truncate if we've exceeded maxWidth
		if sb.Len() > maxWidth && i < len(cube)-1 {
			sb.WriteString(", ...")
			sb.WriteString(fmt.Sprintf("] (%d total)", len(cube)))
			return sb.String()
		}
	}
	sb.WriteString("]")
	return sb.String()
}

func formatCube(cube []int32) string {
	if len(cube) == 0 {
		return "-"
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i, lit := range cube {
		if i > 0 {
			sb.WriteString(",")
		}
		if i >= 3 {
			sb.WriteString("...")
			break
		}
		sb.WriteString(fmt.Sprintf("%d", lit))
	}
	sb.WriteString("]")
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
