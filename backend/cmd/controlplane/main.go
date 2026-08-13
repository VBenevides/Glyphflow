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
	cfg, err := config.FromEnv(config.ControlPlane)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: ":8080", Handler: api.Server{Auth: api.BearerAuthenticator(cfg.APIToken)}.Handler()}
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	fmt.Println("Glyphflow control plane")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
