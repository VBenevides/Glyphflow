package main

import (
	"fmt"
	"os"

	"github.com/VBenevides/Glyphflow/backend/internal/config"
)

func main() {
	if _, err := config.FromEnv(config.Worker); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Glyphflow worker")
}
