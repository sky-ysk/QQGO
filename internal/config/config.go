package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server       ServerConfig
	DBPath       string
	Redis        RedisConfig
	Postgres     PostgresConfig
	NATS         NATSConfig
	MsgRateLimit int
}

type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	MaxConnections  int
	HeartbeatPeriod time.Duration
	TLSCert         string
	TLSKey          string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type PostgresConfig struct {
	DSN string
}

type NATSConfig struct {
	URL string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            env("SERVER_HOST", "0.0.0.0"),
			Port:            envInt("SERVER_PORT", 8080),
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    10 * time.Second,
			MaxConnections:  envInt("MAX_CONNECTIONS", 10000),
			HeartbeatPeriod: 30 * time.Second,
			TLSCert:         env("TLS_CERT", ""),
			TLSKey:          env("TLS_KEY", ""),
		},
		DBPath: env("DB_PATH", "./qqgo.db"),
		Redis: RedisConfig{
			Addr:     env("REDIS_ADDR", "localhost:6379"),
			Password: env("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
		},
		Postgres: PostgresConfig{
			DSN: env("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/qqgo?sslmode=disable"),
		},
		NATS: NATSConfig{
			URL: env("NATS_URL", "nats://localhost:4222"),
		},
		MsgRateLimit: envInt("MSG_RATE_LIMIT", 10),
	}
}

func env(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
