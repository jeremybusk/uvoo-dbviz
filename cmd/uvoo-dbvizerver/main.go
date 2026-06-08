package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"uvoo-dbviz/internal/alert"
	"uvoo-dbviz/internal/auth"
	"uvoo-dbviz/internal/clickhouse"
	"uvoo-dbviz/internal/config"
	"uvoo-dbviz/internal/server"
	"uvoo-dbviz/internal/state"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()
	authn := auth.NewManager(cfg.Auth, http.DefaultClient, logger)
	ch := clickhouse.NewClient(cfg.ClickHouse, http.DefaultClient)
	stateClient := state.NewClient(cfg.PostgREST, http.DefaultClient)
	if cfg.Alerts.Enabled {
		rules, err := alert.RulesFromJSON(cfg.Alerts.Rules)
		if err != nil {
			logger.Error("invalid alert rules", "error", err)
			os.Exit(1)
		}
		worker := alert.NewWorker(cfg.Datasets, cfg.ClickHouse.MaxRows, ch, rules, logger)
		worker.Start(context.Background())
		logger.Info("alert worker started", "rules", len(rules))
	}

	app := server.New(cfg, authn, ch, stateClient, logger)
	logger.Info("starting uvoo-dbviz", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, app); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
