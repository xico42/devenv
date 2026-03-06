package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestLoad_NotifyTelegram(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "telegram"

[notify.telegram]
bot_token = "123:ABC"
chat_id = "456"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "telegram" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "telegram")
	}
	if cfg.Notify.Telegram.BotToken != "123:ABC" {
		t.Errorf("BotToken = %q, want %q", cfg.Notify.Telegram.BotToken, "123:ABC")
	}
	if cfg.Notify.Telegram.ChatID != "456" {
		t.Errorf("ChatID = %q, want %q", cfg.Notify.Telegram.ChatID, "456")
	}
}

func TestLoad_NotifySlack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "slack"

[notify.slack]
webhook_url = "https://hooks.slack.com/services/T/B/X"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "slack" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "slack")
	}
	if cfg.Notify.Slack.WebhookURL != "https://hooks.slack.com/services/T/B/X" {
		t.Errorf("WebhookURL = %q, want expected", cfg.Notify.Slack.WebhookURL)
	}
}

func TestLoad_NotifyDiscord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "discord"

[notify.discord]
webhook_url = "https://discord.com/api/webhooks/123/abc"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "discord" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "discord")
	}
	if cfg.Notify.Discord.WebhookURL != "https://discord.com/api/webhooks/123/abc" {
		t.Errorf("WebhookURL = %q, want expected", cfg.Notify.Discord.WebhookURL)
	}
}

func TestLoad_NotifyWebhook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "webhook"

[notify.webhook]
url = "https://example.com/hook"
method = "PUT"
body_template = "{\"msg\": \"{{.Message}}\"}"

[notify.webhook.headers]
Authorization = "Bearer token123"
Content-Type = "application/json"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "webhook" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "webhook")
	}
	if cfg.Notify.Webhook.URL != "https://example.com/hook" {
		t.Errorf("URL = %q, want expected", cfg.Notify.Webhook.URL)
	}
	if cfg.Notify.Webhook.Method != "PUT" {
		t.Errorf("Method = %q, want %q", cfg.Notify.Webhook.Method, "PUT")
	}
	if cfg.Notify.Webhook.BodyTemplate != `{"msg": "{{.Message}}"}` {
		t.Errorf("BodyTemplate = %q, want expected", cfg.Notify.Webhook.BodyTemplate)
	}
	if cfg.Notify.Webhook.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Authorization header = %q, want expected", cfg.Notify.Webhook.Headers["Authorization"])
	}
	if cfg.Notify.Webhook.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header = %q, want %q", cfg.Notify.Webhook.Headers["Content-Type"], "application/json")
	}
}
