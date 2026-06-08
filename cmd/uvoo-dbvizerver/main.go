package main

import (
	"log/slog"
	"net/http"
	"os"

	"uvoo-dbviz/internal/auth"
	"uvoo-dbviz/internal/clickhouse"
	"uvoo-dbviz/internal/config"
	"uvoo-dbviz/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()
	authn := auth.NewManager(cfg.Auth, http.DefaultClient, logger)
	ch := clickhouse.NewClient(cfg.ClickHouse, http.DefaultClient)

	app := server.New(cfg, authn, ch, logger)
	logger.Info("starting uvoo-dbviz", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, app); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
