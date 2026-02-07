package main

import (
	"flag"
	"log"
	"os"

	"dataminer/internal/app"
)

func main() {
	// Check if first argument is a command (script mode)
	if len(os.Args) > 1 {
		command := os.Args[1]

		// Check if it's a valid script command
		if _, exists := app.ScriptCommands[command]; exists {
			if err := app.RunScriptCommand(command); err != nil {
				log.Fatalf("Command failed: %v", err)
			}
			return
		}

		// Check if it's a valid execution mode
		if isValidMode(command) {
			if err := app.RunScriptMode(command, os.Args[2:]); err != nil {
				log.Fatalf("Mode failed: %v", err)
			}
			return
		}
	}

	// Default behavior: parse flags and run application
	config := app.ParseFlags()

	// Check for help flag or no arguments
	if flag.NFlag() == 0 && len(os.Args) == 1 {
		// No flags provided, show help
		if err := app.ShowUsage(); err != nil {
			log.Fatalf("Failed to show usage: %v", err)
		}
		return
	}

	if err := app.RunApplication(config); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}

// isValidMode checks if the provided string is a valid execution mode
func isValidMode(mode string) bool {
	validModes := []string{
		"production", "test", "optimized", "hybrid",
		"neural", "arxiv", "workflow", "default",
	}
	for _, m := range validModes {
		if m == mode {
			return true
		}
	}
	return false
}
