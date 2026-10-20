package config

import (
	"github.com/joho/godotenv"
	"log"
	"os"
)

type Config struct {
	MonobankApiKey    string `env:"MONOBANK_API_KEY"`
	MonoBankAccountID string `env:"MONO_BANK_ACCOUNT_ID"`

	EzBookKeepingKey     string `env:"EZ_BOOK_KEEPING_KEY"`
	EzBookKeepingBaseURL string `env:"EZ_BOOK_KEEPING_BASE_URL"`

	DBConnection string `env:"DB_CONNECTION" envDefault:"sqlite"`
	DBDatabase   string `env:"DB_DATABASE"`
	DBUsername   string `env:"DB_USERNAME" envDefault:"root"`
	DBPassword   string `env:"DB_PASSWORD" envDefault:"root"`

	BindAddr string `env:"BIND_ADDR" envDefault:":8089"`
	GinMode  string `env:"GIN_MODE" envDefault:"debug"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

func NewConfig() *Config {

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}
	return &Config{
		MonobankApiKey:       os.Getenv("MONOBANK_API_KEY"),
		MonoBankAccountID:    os.Getenv("MONO_BANK_ACCOUNT_ID"),
		EzBookKeepingKey:     os.Getenv("EZ_BOOK_KEEPING_KEY"),
		EzBookKeepingBaseURL: os.Getenv("EZ_BOOK_KEEPING_BASE_URL"),
		DBConnection:         getEnv("DB_CONNECTION", "mu"),
		DBDatabase:           os.Getenv("DB_DATABASE"),
		DBUsername:           getEnv("DB_USERNAME", "root"),
		DBPassword:           getEnv("DB_PASSWORD", "root"),
		BindAddr:             getEnv("BIND_ADDR", ":8089"),
		GinMode:              getEnv("GIN_MODE", "debug"),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
