package config

import "testing"

func TestValidateAllowsDevelopmentDefaults(t *testing.T) {
	cfg := Config{
		PublicURL:  "http://localhost:8080",
		Auth:       AuthConfig{DevMode: true},
		ClickHouse: ClickHouseConfig{URL: "http://localhost:8123"},
		PostgREST:  PostgRESTConfig{URL: "/state"},
		Runtime:    RuntimeConfig{Environment: "development"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestValidateRejectsProductionDefaults(t *testing.T) {
	cfg := Config{
		PublicURL:  "http://localhost:8080",
		Auth:       AuthConfig{DevMode: true},
		ClickHouse: ClickHouseConfig{URL: "http://localhost:8123"},
		PostgREST:  PostgRESTConfig{URL: "/state"},
		Alerts:     AlertConfig{Enabled: true, LoadPersisted: true, WorkerKey: "dev-alert-worker-key"},
		Runtime:    RuntimeConfig{Environment: "production"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production validation error")
	}
}

func TestValidateCanExplicitlyAllowInsecureProductionDefaults(t *testing.T) {
	cfg := Config{
		PublicURL:  "http://localhost:8080",
		Auth:       AuthConfig{DevMode: true},
		ClickHouse: ClickHouseConfig{URL: "http://localhost:8123"},
		PostgREST:  PostgRESTConfig{URL: "/state"},
		Runtime:    RuntimeConfig{Environment: "production", AllowInsecureDefaults: true},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestPublicConfigIncludesAlertDeliveryReadiness(t *testing.T) {
	cfg := Config{
		Auth: AuthConfig{DevMode: true},
		Alerts: AlertConfig{
			Enabled:  true,
			SMTPHost: "smtp.example.com",
			SMTPFrom: "alerts@example.com",
			SMTPUser: "sqviz",
		},
	}

	public := cfg.Public()
	if !public.AlertDelivery.AlertsEnabled {
		t.Fatal("alerts enabled flag was not exposed")
	}
	if !public.AlertDelivery.SMTPConfigured {
		t.Fatal("smtp configured flag was not exposed")
	}
	if !public.AlertDelivery.SMTPHasAuth {
		t.Fatal("smtp auth flag was not exposed")
	}
}
