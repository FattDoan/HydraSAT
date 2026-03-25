package main

import (
	"fmt"
	"strings"
	"time"

	pb "HydraSAT/proto"

	tea "charm.land/bubbletea/v2"
)

type MonitorUI struct {
	client             pb.SolverServiceClient
	masterAddr         string
	stats              *StatsCollector
	width              int
	height             int
	err                error
	masterDisconnected bool
	finalStats         SystemStats
}

func NewMonitorUI(client pb.SolverServiceClient, masterAddr string) *MonitorUI {
	return &MonitorUI{
		client:     client,
		masterAddr: masterAddr,
		stats:      NewStatsCollector(client),
	}
}

type tickMsg time.Time

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *MonitorUI) Init() tea.Cmd {
	return tea.Batch(
		tickEvery(500*time.Millisecond),
		m.stats.FetchStats(),
	)
}

func (m *MonitorUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			// Manual refresh (only if master still connected)
			if !m.masterDisconnected {
				return m, m.stats.FetchStats()
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		// If master disconnected, stop fetching but keep ticking for responsive UI
		if m.masterDisconnected {
			return m, tickEvery(500 * time.Millisecond)
		}
		return m, tea.Batch(
			tickEvery(500*time.Millisecond),
			m.stats.FetchStats(),
		)

	case statsMsg:
		m.stats.Update(msg)
		m.err = nil
		return m, nil

	case errMsg:
		// Check if this is a disconnect error
		errStr := error(msg).Error()
		if strings.Contains(errStr, "Unavailable") ||
			strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "dial tcp") {
			// Master disconnected - save final stats
			m.masterDisconnected = true
			m.finalStats = m.stats.GetStats()
			m.err = nil // Don't show error, show final stats instead
		} else {
			m.err = error(msg)
		}
		return m, nil
	}

	return m, nil
}

func (m *MonitorUI) View() tea.View {
	var content string

	if m.masterDisconnected {
		// Show final stats with completion message
		banner := "╔══════════════════════════════════════════════════════════╗\n"
		banner += "║  ✓ SOLVING COMPLETE - Master Disconnected                ║\n"
		banner += "║  Showing final statistics below                          ║\n"
		banner += "╚══════════════════════════════════════════════════════════╝\n\n"

		// Create a temporary stats collector with final stats for rendering
		tempCollector := &StatsCollector{stats: m.finalStats}
		content = banner + RenderUI(tempCollector, m.masterAddr, m.width, m.height)
		content += "\n\n💡 Press 'q' to quit"

	} else if m.err != nil {
		content = fmt.Sprintf("⚠️  Connection Error\n\n%v\n\nRetrying...\nPress 'q' to quit.", m.err)
	} else {
		content = RenderUI(m.stats, m.masterAddr, m.width, m.height)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
