package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"

	"upd-loader-go/internal/bot"
	"upd-loader-go/internal/config"
)

func main() {
	// Load environment variables from .env file if it exists
	if err := godotenv.Load(); err != nil {
		// .env file is optional, so we don't exit on error
		fmt.Printf("Warning: .env file not found or could not be loaded: %v\n", err)
	}

	// Initialize logger
	logger := setupLogger()

	logger.Info("Starting UPD Loader Bot...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	logger.Info("Configuration loaded successfully")

	// Validate configuration
	if errors := cfg.Validate(); len(errors) > 0 {
		for _, err := range errors {
			logger.Error(err)
		}
		logger.Fatalf("Configuration validation failed")
	}

	logger.Info("Configuration validated successfully")

	// Create and start Telegram bot
	telegramBot, err := bot.NewTelegramUPDBot(cfg, logger)
	if err != nil {
		logger.Fatalf("Failed to create Telegram bot: %v", err)
	}

	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start bot in a goroutine
	go func() {
		if err := telegramBot.Run(); err != nil {
			logger.Fatalf("Bot error: %v", err)
		}
	}()

	logger.Info("UPD Loader Bot started successfully")
	logger.Info("Press Ctrl+C to stop the bot")

	// Wait for shutdown signal
	<-c
	logger.Info("Shutting down UPD Loader Bot...")
	logger.Info("Bot stopped")
}

// setupLogger configures and returns a logger instance
func setupLogger() *logrus.Logger {
	logger := logrus.New()

	// Set log level based on environment
	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	// Set log format
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			ForceColors:     true,
		})
	}

	// Set output
	logger.SetOutput(os.Stdout)

	return logger
}