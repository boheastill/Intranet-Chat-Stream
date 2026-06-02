package bus

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
)

const configPath = "./config.json"

// Config represents the application configuration format.
type Config struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	LoginKey string `json:"login_key"`
}

// generateRandomToken creates a cryptographically secure 32-character token.
func generateRandomToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "default_secure_token_9981"
	}
	return hex.EncodeToString(bytes)
}

// LoadConfig loads config from config.json or generates a new one.
func LoadConfig() Config {
	defaultPassword := "66666666"
	defaultLoginKey := "vip"

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		token := generateRandomToken()
		cfg := Config{
			Token:    token,
			Password: defaultPassword,
			LoginKey: defaultLoginKey,
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(configPath, data, 0600)
		log.Printf("Generated new secure config in config.json with token: %s", token)
		return cfg
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Warning: failed to read config.json: %v. Using defaults.", err)
		return Config{
			Token:    "temporary_token",
			Password: defaultPassword,
			LoginKey: defaultLoginKey,
		}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Token == "" {
		log.Printf("Warning: invalid config.json. Using defaults.")
		return Config{
			Token:    "temporary_token",
			Password: defaultPassword,
			LoginKey: defaultLoginKey,
		}
	}

	if cfg.Password == "" {
		cfg.Password = defaultPassword
	}
	if cfg.LoginKey == "" {
		cfg.LoginKey = defaultLoginKey
	}

	// Save back to config.json if there were missing fields
	if cfg.Password == defaultPassword || cfg.LoginKey == defaultLoginKey {
		data, _ = json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(configPath, data, 0600)
	}

	return cfg
}
