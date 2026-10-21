package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
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

	// Авторизація PWA: логін, bcrypt-хеш пароля (cmd/hashpw) і секрет для підпису токена.
	AuthLogin        string `env:"AUTH_LOGIN"`
	AuthPasswordHash string `env:"AUTH_PASSWORD_HASH"`
	AuthSecret       string `env:"AUTH_SECRET"`

	// Дефолтні категорії — коли жодне правило в mcc_category_rules не підійшло.
	DefaultExpenseCategoryID string `env:"EZ_BOOK_KEEPING_DEFAULT_EXPENSE_CATEGORY_ID"`
	DefaultIncomeCategoryID  string `env:"EZ_BOOK_KEEPING_DEFAULT_INCOME_CATEGORY_ID"`
	// Дефолтний рахунок — поки немає мапінгу account_mappings.
	DefaultSourceAccountID string `env:"EZ_BOOK_KEEPING_DEFAULT_ACCOUNT_ID"`
}

func NewConfig() *Config {

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}
	return &Config{
		MonobankApiKey:           os.Getenv("MONOBANK_API_KEY"),
		MonoBankAccountID:        os.Getenv("MONO_BANK_ACCOUNT_ID"),
		EzBookKeepingKey:         os.Getenv("EZ_BOOK_KEEPING_KEY"),
		EzBookKeepingBaseURL:     os.Getenv("EZ_BOOK_KEEPING_BASE_URL"),
		DBConnection:             getEnv("DB_CONNECTION", "mu"),
		DBDatabase:               os.Getenv("DB_DATABASE"),
		DBUsername:               getEnv("DB_USERNAME", "root"),
		DBPassword:               getEnv("DB_PASSWORD", "root"),
		BindAddr:                 getEnv("BIND_ADDR", ":8089"),
		GinMode:                  getEnv("GIN_MODE", "debug"),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
		DefaultExpenseCategoryID: os.Getenv("EZ_BOOK_KEEPING_DEFAULT_EXPENSE_CATEGORY_ID"),
		DefaultIncomeCategoryID:  os.Getenv("EZ_BOOK_KEEPING_DEFAULT_INCOME_CATEGORY_ID"),
		DefaultSourceAccountID:   os.Getenv("EZ_BOOK_KEEPING_DEFAULT_ACCOUNT_ID"),
		AuthLogin:                os.Getenv("AUTH_LOGIN"),
		AuthPasswordHash:         os.Getenv("AUTH_PASSWORD_HASH"),
		AuthSecret:               os.Getenv("AUTH_SECRET"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
