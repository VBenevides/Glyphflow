package backend

import (
	"os"
	"strings"
)

var Version = "dev"

func init() {
	if raw, err := os.ReadFile("../VERSION"); err == nil {
		if version := strings.TrimSpace(string(raw)); version != "" {
			Version = version
		}
	}
}
