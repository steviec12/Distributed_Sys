package main

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	DataBackend       string
	MySQLDSN          string
	MySQLHost         string
	MySQLPort         string
	MySQLUser         string
	MySQLPassword     string
	MySQLDatabaseName string
	MySQLPool         MySQLPoolConfig
	AWSRegion         string
	DynamoDBTableName string
}

func LoadConfig() Config {
	return Config{
		Port:              getEnv("PORT", "8080"),
		DataBackend:       getEnv("DATA_BACKEND", "mysql"),
		MySQLDSN:          os.Getenv("MYSQL_DSN"),
		MySQLHost:         getEnv("MYSQL_HOST", "127.0.0.1"),
		MySQLPort:         getEnv("MYSQL_PORT", "3306"),
		MySQLUser:         getEnv("MYSQL_USER", "root"),
		MySQLPassword:     os.Getenv("MYSQL_PASSWORD"),
		MySQLDatabaseName: getEnv("MYSQL_DATABASE_NAME", "hw8"),
		MySQLPool: MySQLPoolConfig{
			MaxOpenConns:    getEnvInt("MYSQL_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getEnvInt("MYSQL_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("MYSQL_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getEnvDuration("MYSQL_CONN_MAX_IDLE_TIME", 2*time.Minute),
		},
		AWSRegion:         getEnv("AWS_REGION", "us-west-2"),
		DynamoDBTableName: getEnv("DYNAMODB_TABLE_NAME", "shopping_carts"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
