//go:build workerui_tui && !workerui && !workerui_fyne && !workerui_gio

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	tuiFilterRow = 7
	tuiLogStart  = 9
)

type tuiTick time.Time

type tuiModel struct {
	logs     *LogBuffer
	cancel   context.CancelFunc
	snapshot Snapshot
	entries  []LogEntry
	after    uint64
	offset   int
	width    int
	height   int
	stderr   bool
}

func newTUIModel(logs *LogBuffer, cancel context.CancelFunc) tuiModel {
	return tuiModel{logs: logs, cancel: cancel, width: 80, height: 24}
}

func (m tuiModel) Init() tea.Cmd {
	return tuiPoll()
}

func tuiPoll() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(now time.Time) tea.Msg { return tuiTick(now) })
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tuiTick:
		return m.updateTick()
	case tea.KeyMsg:
		if cmd := m.updateKey(msg); cmd != nil {
			return m, cmd
		}
	case tea.MouseMsg:
		m.updateMouse(msg)
	}
	return m, nil
}

func (m tuiModel) updateTick() (tea.Model, tea.Cmd) {
	snapshot := m.logs.Snapshot(m.after)
	if snapshot.Reset {
		m.entries = append([]LogEntry(nil), snapshot.Entries...)
	} else {
		m.entries = append(m.entries, snapshot.Entries...)
	}
	if len(m.entries) > maxWorkerLogEntries {
		m.entries = m.entries[len(m.entries)-maxWorkerLogEntries:]
	}
	if len(snapshot.Entries) > 0 {
		m.after = snapshot.Entries[len(snapshot.Entries)-1].Sequence
	}
	m.snapshot = snapshot
	setTUITrayTooltip(snapshot)
	m.offset = min(m.offset, m.maxScroll())
	return m, tuiPoll()
}

func (m *tuiModel) updateKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Quit
	case "a":
		m.stderr = false
		m.offset = 0
	case "tab":
		m.stderr = !m.stderr
		m.offset = 0
	case "e":
		m.stderr = true
		m.offset = 0
	case "up", "k":
		m.offset = min(m.offset+1, m.maxScroll())
	case "down", "j":
		m.offset = max(m.offset-1, 0)
	case "pgup":
		m.offset = min(m.offset+m.logHeight(), m.maxScroll())
	case "pgdown":
		m.offset = max(m.offset-m.logHeight(), 0)
	case "home":
		m.offset = m.maxScroll()
	case "end":
		m.offset = 0
	}
	return nil
}

func (m *tuiModel) updateMouse(msg tea.MouseMsg) {
	if msg.Button == tea.MouseButtonWheelUp {
		m.offset = min(m.offset+3, m.maxScroll())
	} else if msg.Button == tea.MouseButtonWheelDown {
		m.offset = max(m.offset-3, 0)
	} else if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == tuiFilterRow {
		m.stderr = msg.X >= 20
		m.offset = 0
	}
}

func (m tuiModel) View() string {
	width := max(m.width, 1)
	lines := []string{
		"Glyphflow Worker | TUI",
		fmt.Sprintf("Runner ID: %s", tuiValue(m.snapshot.RunnerID)),
		fmt.Sprintf("NATS: %s", tuiValue(m.snapshot.NATSEndpoint)),
		fmt.Sprintf("Current executions: %d", m.snapshot.RunningExecutions),
		fmt.Sprintf("Parallel executions capacity: %d", m.snapshot.ParallelExecutions),
		"",
		"Mouse wheel/keys scroll logs; click or press A/E to filter.",
		"",
		fmt.Sprintf("Filter: [%s] All    [%s] Stderr", tuiMark(!m.stderr), tuiMark(m.stderr)),
		strings.Repeat("-", width),
	}

	visible := m.visibleEntries()
	height := m.logHeight()
	start := max(len(visible)-height-m.offset, 0)
	end := min(start+height, len(visible))
	for _, entry := range visible[start:end] {
		lines = append(lines, fmt.Sprintf("%s %s", entry.Timestamp, entry.Text))
	}
	for len(lines) < max(m.height-1, tuiLogStart+1) {
		lines = append(lines, "")
	}
	lines = append(lines, "q/Ctrl-C quit | A all | E stderr | ↑/↓/PgUp/PgDn/Home/End scroll")
	for i := range lines {
		lines[i] = tuiFit(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func (m tuiModel) visibleEntries() []LogEntry {
	visible := make([]LogEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		if !m.stderr || entry.Stream == "stderr" {
			visible = append(visible, entry)
		}
	}
	return visible
}

func (m tuiModel) logHeight() int {
	return max(m.height-tuiLogStart-1, 1)
}

func (m tuiModel) maxScroll() int {
	return max(len(m.visibleEntries())-m.logHeight(), 0)
}

func tuiValue(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func tuiMark(active bool) string {
	if active {
		return "*"
	}
	return " "
}

func tuiFit(value string, width int) string {
	runes := []rune(value)
	if len(runes) > width {
		return string(runes[:max(width-1, 0)]) + "…"
	}
	return value + strings.Repeat(" ", width-len(runes))
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	var capacity atomic.Int64
	logs := NewLogBuffer(&capacity)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := runWorker(ctx, logs.Writer("stdout", nil), logs.Writer("stderr", nil), logs); err != nil {
			_, _ = logs.Writer("stderr", nil).Write([]byte(err.Error() + "\n"))
		}
	}()

	program := tea.NewProgram(newTUIModel(logs, cancel), tea.WithAltScreen(), tea.WithMouseCellMotion())
	stopTray := startTUITray(func() {
		cancel()
		program.Quit()
	})
	defer stopTray()
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	cancel()
	<-workerDone
}
