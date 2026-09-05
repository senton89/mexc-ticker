package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — параметры сервиса из переменных окружения.
type Config struct {
	Symbols       []string
	DatabaseURL   string
	HTTPAddr      string
	BatchSize     int
	FlushInterval time.Duration
	PollInterval  time.Duration
	MEXCBaseURL   string
}

// Load читает env, подставляет значения по умолчанию и валидирует.
func Load() (Config, error) {
	cfg := Config{
		Symbols:       splitCSV(getenv("SYMBOLS", "BTCUSDT,ETHUSDT")),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		MEXCBaseURL:   getenv("MEXC_BASE_URL", "https://api.mexc.com"),
	}

	var err error
	if cfg.BatchSize, err = strconv.Atoi(getenv("BATCH_SIZE", "100")); err != nil {
		return Config{}, fmt.Errorf("config: BATCH_SIZE: %w", err)
	}
	if cfg.FlushInterval, err = time.ParseDuration(getenv("FLUSH_INTERVAL", "5s")); err != nil {
		return Config{}, fmt.Errorf("config: FLUSH_INTERVAL: %w", err)
	}
	if cfg.PollInterval, err = time.ParseDuration(getenv("POLL_INTERVAL", "2s")); err != nil {
		return Config{}, fmt.Errorf("config: POLL_INTERVAL: %w", err)
	}

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch {
	case c.DatabaseURL == "":
		return errors.New("config: DATABASE_URL is required")
	case len(c.Symbols) == 0:
		return errors.New("config: SYMBOLS is empty")
	case len(c.Symbols) > 30:
		return errors.New("config: more than 30 symbols (MEXC limit per connection)")
	case c.BatchSize <= 0:
		return errors.New("config: BATCH_SIZE must be > 0")
	case c.FlushInterval <= 0 || c.PollInterval <= 0:
		return errors.New("config: intervals must be > 0")
	case float64(len(c.Symbols))*float64(time.Second)/float64(c.PollInterval) > 10:
		return errors.New("config: SYMBOLS/POLL_INTERVAL exceeds ~10 rps (MEXC IP rate limit)")
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(strings.ToUpper(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}