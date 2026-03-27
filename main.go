package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var vaultPath string
	var configPath string

	flag.StringVar(&vaultPath, "vault", "", "Path to Obsidian vault")
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if vaultPath != "" {
		cfg.Vault.Path = vaultPath
	}

	if cfg.Vault.Path == "" {
		fmt.Fprintln(os.Stderr, "No vault path configured. Set it in ~/.config/obsidian-tasks/config.toml or use --vault flag.")
		os.Exit(1)
	}

	cache, tasks, err := NewDailyNotesCache(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning tasks: %v\n", err)
		os.Exit(1)
	}

	model := NewModel(cfg, cache, tasks)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
