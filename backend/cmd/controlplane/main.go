package main

import (
	"fmt"
	"os"

	"github.com/VBenevides/Glyphflow/backend/internal/config"
)

func main() {
	if _, err := config.FromEnv(config.ControlPlane); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Glyphflow control plane")
}
