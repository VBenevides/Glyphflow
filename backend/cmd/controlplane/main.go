package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/VBenevides/Glyphflow/backend/internal/api"
	"github.com/VBenevides/Glyphflow/backend/internal/config"
)

func main() {
	if _, err := config.FromEnv(config.ControlPlane); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: ":8080", Handler: api.Server{Auth: func(*http.Request) (api.Claims, bool) {
		return api.Claims{Subject: "local", Roles: map[string]bool{"task.read": true}}, true
	}}.Handler()}
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	fmt.Println("Glyphflow control plane")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
