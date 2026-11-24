package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ecakeman/alert/internal/bootstrap"
	"github.com/ecakeman/alert/internal/config"
)

func main() {
	migrateOnly := flag.Bool("migrate", false, "run migrations then exit")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Printf("config: %v", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *migrateOnly {
		if err := bootstrap.RunMigrate(ctx, cfg); err != nil {
			log.Printf("migrate: %v", err)
			os.Exit(1)
		}
		return
	}

	sh, err := bootstrap.NewShared(ctx, cfg)
	if err != nil {
		log.Printf("startup: %v", err)
		os.Exit(1)
	}
	repos := bootstrap.NewRepos(sh)
	ucs := bootstrap.NewUseCases(sh, repos)

	api, err := bootstrap.NewAPI(sh, repos, ucs)
	if err != nil {
		log.Printf("startup: %v", err)
		os.Exit(1)
	}
	worker, err := bootstrap.NewWorker(sh, repos, ucs)
	if err != nil {
		log.Printf("startup: %v", err)
		os.Exit(1)
	}

	if err := bootstrap.RunCombined(ctx, api, worker, sh); err != nil {
		log.Printf("server: %v", err)
		os.Exit(1)
	}
}
