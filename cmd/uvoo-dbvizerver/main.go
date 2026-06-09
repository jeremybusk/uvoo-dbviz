package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

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
		staticRules, err := alert.RulesFromJSON(cfg.Alerts.Rules)
		if err != nil {
			logger.Error("invalid alert rules", "error", err)
			os.Exit(1)
		}
		loader := func(ctx context.Context) ([]alert.Rule, error) {
			rules := append([]alert.Rule{}, staticRules...)
			if cfg.Alerts.LoadPersisted {
				rows, err := stateClient.LoadEnabledAlertRules(ctx, cfg.Alerts.WorkerKey)
				if err != nil {
					return rules, err
				}
				persisted, err := alert.RulesFromPersisted(rows)
				if err != nil {
					return rules, err
				}
				rules = append(rules, persisted...)
			}
			return rules, nil
		}
		worker := alert.NewPollingWorker(
			cfg.Datasets,
			cfg.ClickHouse.MaxRows,
			ch,
			loader,
			time.Duration(cfg.Alerts.PollSeconds)*time.Second,
			logger,
		)
		worker.SetDedupeWindow(time.Duration(cfg.Alerts.DedupeSeconds) * time.Second)
		worker.SetIncidentRecorder(func(ctx context.Context, rule alert.Rule, status string, value float64, payload map[string]any, fingerprint string, cooldownSeconds int) (alert.RecordResult, error) {
			incident, err := stateClient.RecordAlertIncident(ctx, cfg.Alerts.WorkerKey, rule.ID, rule.TenantID, status, value, payload, fingerprint, cooldownSeconds)
			if err != nil {
				return alert.RecordResult{}, err
			}
			return alert.RecordResult{Deduped: incident.Deduped, ShouldNotify: incident.ShouldNotify}, nil
		})
		worker.Start(context.Background())
		logger.Info("alert worker started", "static_rules", len(staticRules), "load_persisted", cfg.Alerts.LoadPersisted)
	}

	app := server.New(cfg, authn, ch, stateClient, logger)
	logger.Info("starting uvoo-dbviz", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, app); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
