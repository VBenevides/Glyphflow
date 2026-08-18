//go:build workerui_tui && !workerui && !workerui_fyne && !workerui_gio

package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIKeyboardAndMouseControls(t *testing.T) {
	logs := NewLogBuffer(nil)
	for i := 0; i < 25; i++ {
		_, _ = logs.Writer("stdout", nil).Write([]byte("stdout\n"))
		_, _ = logs.Writer("stderr", nil).Write([]byte("stderr\n"))
	}
	model := newTUIModel(logs, nil)
	model.height = 12
	updated, _ := model.Update(tuiTick{})
	model = updated.(tuiModel)
	if model.maxScroll() == 0 {
		t.Fatal("expected scrollable log history")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(tuiModel)
	if !model.stderr || len(model.visibleEntries()) != 25 {
		t.Fatalf("keyboard filter failed: stderr=%v visible=%d", model.stderr, len(model.visibleEntries()))
	}

	updated, _ = model.Update(tea.MouseMsg(tea.MouseEvent{X: 2, Y: tuiFilterRow, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}))
	model = updated.(tuiModel)
	if model.stderr {
		t.Fatal("mouse All filter failed")
	}
	before := model.offset
	updated, _ = model.Update(tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}))
	model = updated.(tuiModel)
	if model.offset <= before {
		t.Fatal("mouse wheel did not scroll")
	}
}
