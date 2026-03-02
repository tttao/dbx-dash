package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines all global key bindings.
type keyMap struct {
	Dashboard  key.Binding
	Jobs       key.Binding
	Clusters   key.Binding
	Warehouses key.Binding
	Identity   key.Binding
	Catalogs   key.Binding
	Refresh    key.Binding
	NextWS     key.Binding
	Select     key.Binding
	Back       key.Binding
	Quit       key.Binding
}

var keys = keyMap{
	Dashboard: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "dashboard"),
	),
	Jobs: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "jobs"),
	),
	Clusters: key.NewBinding(
		key.WithKeys("3"),
		key.WithHelp("3", "clusters"),
	),
	Warehouses: key.NewBinding(
		key.WithKeys("4"),
		key.WithHelp("4", "warehouses"),
	),
	Identity: key.NewBinding(
		key.WithKeys("5"),
		key.WithHelp("5", "identity"),
	),
	Catalogs: key.NewBinding(
		key.WithKeys("6"),
		key.WithHelp("6", "catalogs"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	NextWS: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "next workspace"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "b"),
		key.WithHelp("esc/b", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
