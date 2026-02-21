package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Ollama   OllamaConfig   `yaml:"ollama"`
	Telegram TelegramConfig `yaml:"telegram"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type OllamaConfig struct {
	Address  string `yaml:"address"`
	Model    string `yaml:"model"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func Load(path string) (*Config, error) {
	loadEnvFile(".env")

	cfg := &Config{
		Ollama: OllamaConfig{
			Address: "http://localhost:11434",
			Model:   "gemma3:4b",
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnv(cfg)
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	applyEnv(cfg)
	return cfg, nil
}

// applyEnv overrides config fields with environment variables when set.
// Env vars take precedence over YAML config values.
func applyEnv(cfg *Config) {
	if v := os.Getenv("OLLAMA_ADDRESS"); v != "" {
		cfg.Ollama.Address = v
	}
	if v := os.Getenv("OLLAMA_MODEL"); v != "" {
		cfg.Ollama.Model = v
	}
	if v := os.Getenv("OLLAMA_USERNAME"); v != "" {
		cfg.Ollama.Username = v
	}
	if v := os.Getenv("OLLAMA_PASSWORD"); v != "" {
		cfg.Ollama.Password = v
	}
	if v := os.Getenv("TG_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("TG_CHAT_ID"); v != "" {
		cfg.Telegram.ChatID = v
	}
}

// loadEnvFile reads a .env file and sets environment variables
// that are not already set in the process environment.
func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		// Only set if not already in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
