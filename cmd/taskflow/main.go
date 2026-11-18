package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"taskflow/internal/mcp"
	"taskflow/internal/repository"
	"taskflow/internal/tui"
)

func main() {
	dbPath := os.Getenv("TASKFLOW_DB")
	if strings.TrimSpace(dbPath) == "" {
		dbPath = "taskflow.db"
	}

	// Detect `taskflow mcp` subcommand (SPEC-001 §2)
	isMCPMode := false
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "mcp" {
		isMCPMode = true
		args = args[1:]
	}

	fs := flag.NewFlagSet("taskflow", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&dbPath, "db", dbPath, "Ruta al archivo SQLite (por defecto: taskflow.db o $TASKFLOW_DB)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if !isMCPMode && fs.NArg() > 0 && fs.Arg(0) == "mcp" {
		isMCPMode = true
	}

	// Ensure logs never pollute stdout (critical for both MCP JSON-RPC and TUI rendering)
	log.SetOutput(os.Stderr)

	db, err := repository.NewSQLiteDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error inicializando SQLite (%s): %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	repo := repository.NewTaskRepository(db)

	if isMCPMode {
		srv := mcp.NewServer(repo)
		if err := srv.ServeStdio(); err != nil {
			fmt.Fprintf(os.Stderr, "error en servidor MCP: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// TUI mode: silence standard logger so background logs don't corrupt alternate screen
	log.SetOutput(io.Discard)
	model := tui.NewModel(repo)
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error ejecutando TUI: %v\n", err)
		os.Exit(1)
	}
}
