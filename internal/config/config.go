package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	// Telegram Bot
	TelegramBotToken string
	AuthorizedUsers  []int64

	// MoySkald API
	MoySkladAPIToken      string
	MoySkladAPIURL        string
	MoySkladOrganizationID string

	// Application settings
	TempDir     string
	LogLevel    string
	MaxFileSize int64

	// UPD file encoding
	UPDEncoding string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	config := &Config{
		TelegramBotToken:       os.Getenv("TELEGRAM_BOT_TOKEN"),
		MoySkladAPIToken:       os.Getenv("MOYSKLAD_API_TOKEN"),
		MoySkladAPIURL:         getEnvWithDefault("MOYSKLAD_API_URL", "https://api.moysklad.ru/api/remap/1.2"),
		MoySkladOrganizationID: os.Getenv("MOYSKLAD_ORGANIZATION_ID"),
		TempDir:                getEnvWithDefault("TEMP_DIR", "./temp"),
		LogLevel:               getEnvWithDefault("LOG_LEVEL", "INFO"),
		UPDEncoding:            "windows-1251",
	}

	// Parse authorized users
	usersStr := os.Getenv("AUTHORIZED_USERS")
	if usersStr != "" {
		userIDs := strings.Split(usersStr, ",")
		for _, userIDStr := range userIDs {
			userIDStr = strings.TrimSpace(userIDStr)
			if userIDStr != "" {
				userID, err := strconv.ParseInt(userIDStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid user ID: %s", userIDStr)
				}
				config.AuthorizedUsers = append(config.AuthorizedUsers, userID)
			}
		}
	}

	// Parse max file size
	maxFileSizeStr := getEnvWithDefault("MAX_FILE_SIZE", "10485760") // 10MB
	maxFileSize, err := strconv.ParseInt(maxFileSizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_FILE_SIZE: %s", maxFileSizeStr)
	}
	config.MaxFileSize = maxFileSize

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() []string {
	var errors []string

	if c.TelegramBotToken == "" {
		errors = append(errors, "TELEGRAM_BOT_TOKEN не установлен")
	}

	if c.MoySkladAPIToken == "" {
		errors = append(errors, "MOYSKLAD_API_TOKEN не установлен")
	}

	if len(c.AuthorizedUsers) == 0 {
		errors = append(errors, "AUTHORIZED_USERS не установлены")
	}

	return errors
}

// EnsureTempDir creates the temporary directory if it doesn't exist
func (c *Config) EnsureTempDir() error {
	return os.MkdirAll(c.TempDir, 0755)
}

// IsAuthorizedUser checks if the user ID is authorized
func (c *Config) IsAuthorizedUser(userID int64) bool {
	for _, id := range c.AuthorizedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}