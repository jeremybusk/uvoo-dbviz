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
	"uvoo-dbviz/internal/secrets"
	"uvoo-dbviz/internal/server"
	"uvoo-dbviz/internal/state"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
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
		worker.SetSMTP(alert.SMTPConfig{
			Host:     cfg.Alerts.SMTPHost,
			Port:     cfg.Alerts.SMTPPort,
			User:     cfg.Alerts.SMTPUser,
			Password: cfg.Alerts.SMTPPassword,
			From:     cfg.Alerts.SMTPFrom,
		})
		worker.SetSecretResolver(func(ctx context.Context, tenantSlug string, secretName string) (string, bool) {
			if cfg.Secrets.EncryptionKey != "" && stateClient.Enabled() {
				row, err := stateClient.GetTenantSecretForWorker(ctx, cfg.Alerts.WorkerKey, tenantSlug, secretName)
				if err == nil {
					value, decryptErr := secrets.DecryptString(row.Ciphertext, row.Nonce, cfg.Secrets.EncryptionKey)
					if decryptErr == nil {
						return value, true
					}
					logger.Warn("tenant secret decrypt failed", "tenant", tenantSlug, "secret", secretName, "error", decryptErr)
				}
			}
			return alert.ResolveSecretRefFromEnv(ctx, tenantSlug, secretName)
		})
		worker.SetIncidentRecorder(func(ctx context.Context, rule alert.Rule, status string, value float64, payload map[string]any, fingerprint string, cooldownSeconds int) (alert.RecordResult, error) {
			incident, err := stateClient.RecordAlertIncident(ctx, cfg.Alerts.WorkerKey, rule.ID, rule.TenantID, status, value, payload, fingerprint, cooldownSeconds)
			if err != nil {
				return alert.RecordResult{}, err
			}
			lastSynced := ""
			if incident.ExternalLastSyncedAt != nil {
				lastSynced = *incident.ExternalLastSyncedAt
			}
			return alert.RecordResult{
				IncidentID:           incident.ID,
				Deduped:              incident.Deduped,
				ShouldNotify:         incident.ShouldNotify,
				ExternalProvider:     incident.ExternalProvider,
				ExternalIncidentID:   incident.ExternalIncidentID,
				ExternalIncidentURL:  incident.ExternalIncidentURL,
				ExternalSyncStatus:   incident.ExternalSyncStatus,
				ExternalLastSyncedAt: lastSynced,
			}, nil
		})
		worker.SetIncidentSyncRecorder(func(ctx context.Context, rule alert.Rule, incidentID string, result alert.DeliveryResult) error {
			_, err := stateClient.UpdateAlertIncidentSync(ctx, cfg.Alerts.WorkerKey, rule.TenantID, incidentID, result.ExternalProvider, result.ExternalIncidentID, result.ExternalIncidentURL, result.ExternalSyncStatus, result.Error)
			return err
		})
		worker.SetPagerDutyReconciler(
			func(ctx context.Context) ([]state.PagerDutySyncedIncident, error) {
				return stateClient.ListPagerDutySyncedIncidents(ctx, cfg.Alerts.WorkerKey)
			},
			func(ctx context.Context, incident state.PagerDutySyncedIncident, remote alert.PagerDutyRemoteIncident, result alert.DeliveryResult) error {
				status := remote.Status
				if result.Status == "failed" {
					status = ""
				}
				_, err := stateClient.ReconcilePagerDutyIncident(ctx, cfg.Alerts.WorkerKey, incident.TenantID, incident.ID, status, result.ExternalIncidentURL, result.ExternalSyncStatus, result.Error)
				return err
			},
		)
		worker.SetNotificationRecorder(func(ctx context.Context, rule alert.Rule, incidentID string, contact alert.ContactEndpoint, result alert.DeliveryResult, payload map[string]any) error {
			_, err := stateClient.RecordAlertNotification(ctx, cfg.Alerts.WorkerKey, rule.ID, rule.TenantID, incidentID, contact.Kind, contact.Target, result.Status, result.StatusCode, result.Error, payload)
			return err
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
