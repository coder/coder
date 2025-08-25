package cliui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/serpent"
)

func TestSelect(t *testing.T) {
	t.Parallel()
	t.Run("Select", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		msgChan := make(chan string)
		go func() {
			resp, err := newSelect(ptty, cliui.SelectOptions{
				Options: []string{"First", "Second"},
			})
			assert.NoError(t, err)
			msgChan <- resp
		}()
		require.Equal(t, "First", <-msgChan)
	})
}

func newSelect(ptty *ptytest.PTY, opts cliui.SelectOptions) (string, error) {
	value := ""
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			var err error
			value, err = cliui.Select(inv, opts)
			return err
		},
	}
	inv := cmd.Invoke()
	ptty.Attach(inv)
	return value, inv.Run()
}

func TestRichSelect(t *testing.T) {
	t.Parallel()
	t.Run("RichSelect", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		msgChan := make(chan string)
		go func() {
			resp, err := newRichSelect(ptty, cliui.RichSelectOptions{
				Options: []codersdk.TemplateVersionParameterOption{
					{Name: "A-Name", Value: "A-Value", Description: "A-Description."},
					{Name: "B-Name", Value: "B-Value", Description: "B-Description."},
				},
			})
			assert.NoError(t, err)
			msgChan <- resp
		}()
		require.Equal(t, "A-Value", <-msgChan)
	})
}

func newRichSelect(ptty *ptytest.PTY, opts cliui.RichSelectOptions) (string, error) {
	value := ""
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			richOption, err := cliui.RichSelect(inv, opts)
			if err == nil {
				value = richOption.Value
			}
			return err
		},
	}
	inv := cmd.Invoke()
	ptty.Attach(inv)
	return value, inv.Run()
}

func TestRichMultiSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		options     []codersdk.TemplateVersionParameterOption
		defaults    []string
		allowCustom bool
		want        []string
	}{
		{
			name: "Predefined",
			options: []codersdk.TemplateVersionParameterOption{
				{Name: "AAA", Description: "This is AAA", Value: "aaa"},
				{Name: "BBB", Description: "This is BBB", Value: "bbb"},
				{Name: "CCC", Description: "This is CCC", Value: "ccc"},
			},
			defaults:    []string{"bbb", "ccc"},
			allowCustom: false,
			want:        []string{"bbb", "ccc"},
		},
		{
			name: "Custom",
			options: []codersdk.TemplateVersionParameterOption{
				{Name: "AAA", Description: "This is AAA", Value: "aaa"},
				{Name: "BBB", Description: "This is BBB", Value: "bbb"},
				{Name: "CCC", Description: "This is CCC", Value: "ccc"},
			},
			defaults:    []string{"aaa", "bbb"},
			allowCustom: true,
			want:        []string{"aaa", "bbb"},
		},
		{
			name: "NoOptionSelected",
			options: []codersdk.TemplateVersionParameterOption{
				{Name: "AAA", Description: "This is AAA", Value: "aaa"},
				{Name: "BBB", Description: "This is BBB", Value: "bbb"},
				{Name: "CCC", Description: "This is CCC", Value: "ccc"},
			},
			defaults:    []string{},
			allowCustom: false,
			want:        []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var selectedItems []string
			var err error
			cmd := &serpent.Command{
				Handler: func(inv *serpent.Invocation) error {
					selectedItems, err = cliui.RichMultiSelect(inv, cliui.RichMultiSelectOptions{
						Options:           tt.options,
						Defaults:          tt.defaults,
						EnableCustomInput: tt.allowCustom,
					})
					return err
				},
			}

			doneChan := make(chan struct{})
			go func() {
				defer close(doneChan)
				err := cmd.Invoke().Run()
				assert.NoError(t, err)
			}()
			<-doneChan

			require.Equal(t, tt.want, selectedItems)
		})
	}
}

func TestMultiSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		items       []string
		allowCustom bool
		want        []string
	}{
		{
			name:        "MultiSelect",
			items:       []string{"aaa", "bbb", "ccc"},
			allowCustom: false,
			want:        []string{"aaa", "bbb", "ccc"},
		},
		{
			name:        "MultiSelectWithCustomInput",
			items:       []string{"Code", "Chairs", "Whale", "Diamond", "Carrot"},
			allowCustom: true,
			want:        []string{"Code", "Chairs", "Whale", "Diamond", "Carrot"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ptty := ptytest.New(t)
			msgChan := make(chan []string)

			go func() {
				resp, err := newMultiSelect(ptty, tt.items, tt.allowCustom)
				assert.NoError(t, err)
				msgChan <- resp
			}()

			require.Equal(t, tt.want, <-msgChan)
		})
	}
}

func newMultiSelect(pty *ptytest.PTY, items []string, custom bool) ([]string, error) {
	var values []string
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			selectedItems, err := cliui.MultiSelect(inv, cliui.MultiSelectOptions{
				Options:           items,
				Defaults:          items,
				EnableCustomInput: custom,
			})
			if err == nil {
				values = selectedItems
			}
			return err
		},
	}
	inv := cmd.Invoke()
	pty.Attach(inv)
	return values, inv.Run()
}
