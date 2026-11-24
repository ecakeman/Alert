package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ecakeman/alert/services/system-core/internal/bootstrap"
	"github.com/ecakeman/alert/services/system-core/internal/config"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Printf("config: %v", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sh, err := bootstrap.NewShared(ctx, cfg)
	if err != nil {
		log.Printf("startup: %v", err)
		os.Exit(1)
	}
	repos := bootstrap.NewRepos(sh)
	ucs := bootstrap.NewUseCases(sh, repos)

	worker, err := bootstrap.NewWorker(sh, repos, ucs)
	if err != nil {
		log.Printf("startup: %v", err)
		os.Exit(1)
	}

	if err := bootstrap.RunWorker(ctx, worker, sh); err != nil {
		log.Printf("worker: %v", err)
		os.Exit(1)
	}
}
