package config

import (
	"errors"
	"io/fs"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	// MonobankApiKey — лише для ручної реєстрації вебхука (POST /personal/webhook);
	// для перевірки підпису він не потрібен, ключ банку публічний.
	MonobankApiKey string `env:"MONOBANK_API_KEY"`
	// MonoSpoolURL — черга вебхуків на VPS, які прийшли, поки сервер лежав; порожньо — не забирати.
	MonoSpoolURL string `env:"MONO_SPOOL_URL"`

	EzBookKeepingKey     string `env:"EZ_BOOK_KEEPING_KEY"`
	EzBookKeepingBaseURL string `env:"EZ_BOOK_KEEPING_BASE_URL"`

	// DBDatabase — шлях до файлу SQLite відносно робочого каталогу
	DBDatabase string `env:"DB_DATABASE"`

	BindAddr string `env:"BIND_ADDR" envDefault:":8089"`
	GinMode  string `env:"GIN_MODE" envDefault:"debug"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// Авторизація PWA: логін, bcrypt-хеш пароля (cmd/hashpw) і секрет для підпису токена.
	AuthLogin        string `env:"AUTH_LOGIN"`
	AuthPasswordHash string `env:"AUTH_PASSWORD_HASH"`
	AuthSecret       string `env:"AUTH_SECRET"`
}

func NewConfig() *Config {

	// .env шукається в поточному каталозі процесу; під systemd це WorkingDirectory.
	// Відсутній файл і зламаний файл — різні проблеми, тому й повідомлення різні:
	// раніше обидві давали "not found", і синтаксична помилка ховалась.
	if err := godotenv.Load(); err != nil {
		wd, _ := os.Getwd()
		if errors.Is(err, fs.ErrNotExist) {
			log.Printf(".env не знайдено в %s — беремо змінні лише з оточення", wd)
		} else {
			log.Printf(".env у %s не вдалося прочитати: %v", wd, err)
		}
	}
	return &Config{
		MonobankApiKey:       os.Getenv("MONOBANK_API_KEY"),
		MonoSpoolURL:         os.Getenv("MONO_SPOOL_URL"),
		EzBookKeepingKey:     os.Getenv("EZ_BOOK_KEEPING_KEY"),
		EzBookKeepingBaseURL: os.Getenv("EZ_BOOK_KEEPING_BASE_URL"),
		DBDatabase:           os.Getenv("DB_DATABASE"),
		BindAddr:             getEnv("BIND_ADDR", ":8089"),
		GinMode:              getEnv("GIN_MODE", "debug"),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		AuthLogin:            os.Getenv("AUTH_LOGIN"),
		AuthPasswordHash:     os.Getenv("AUTH_PASSWORD_HASH"),
		AuthSecret:           os.Getenv("AUTH_SECRET"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
