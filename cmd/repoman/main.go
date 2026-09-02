package main

import (
	"os"

	"github.com/klokie/repoman/internal/cmd"
)

func main() {
	// Cobra has already printed the error; repeating it here doubled every
	// failure line in the scheduled-job logs.
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
