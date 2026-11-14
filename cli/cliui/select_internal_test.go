package cliui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestMultiSelectModelEnterFinishesEmptyOptions(t *testing.T) {
	t.Parallel()

	model := multiSelectModel{}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result, ok := updated.(multiSelectModel)
	require.True(t, ok)
	require.True(t, result.selected)
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, cmd())
	require.Empty(t, result.selectedOptions())
}

func TestMultiSelectModelViewExplainsCompletion(t *testing.T) {
	t.Parallel()

	model := multiSelectModel{search: textinput.New()}
	view := model.View()
	require.Contains(t, view, "<enter> to finish")
	require.Contains(t, view, "<ctrl+c> to cancel")
}
