package config

// NotifyConfig holds notification provider settings.
type NotifyConfig struct {
	Provider string               `toml:"provider"`
	Telegram TelegramNotifyConfig `toml:"telegram"`
	Slack    SlackNotifyConfig    `toml:"slack"`
	Discord  DiscordNotifyConfig  `toml:"discord"`
	Webhook  WebhookNotifyConfig  `toml:"webhook"`
}

// TelegramNotifyConfig holds Telegram bot settings.
type TelegramNotifyConfig struct {
	BotToken string `toml:"bot_token" secret:"true"`
	ChatID   string `toml:"chat_id"   secret:"true"`
}

// SlackNotifyConfig holds Slack webhook settings.
type SlackNotifyConfig struct {
	WebhookURL string `toml:"webhook_url" secret:"true"`
}

// DiscordNotifyConfig holds Discord webhook settings.
type DiscordNotifyConfig struct {
	WebhookURL string `toml:"webhook_url" secret:"true"`
}

// WebhookNotifyConfig holds generic webhook settings.
type WebhookNotifyConfig struct {
	URL          string            `toml:"url"           secret:"true"`
	Method       string            `toml:"method"`
	Headers      map[string]string `toml:"headers"`
	BodyTemplate string            `toml:"body_template"`
}
