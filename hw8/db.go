package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func DefaultMySQLPoolConfig() MySQLPoolConfig {
	return MySQLPoolConfig{
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}
}

func OpenMySQLDB(cfg Config) (*sql.DB, error) {
	dsn, err := resolveMySQLDSN(cfg)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	applyMySQLPoolConfig(db, cfg.MySQLPool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}

func resolveMySQLDSN(cfg Config) (string, error) {
	if cfg.MySQLDSN != "" {
		return cfg.MySQLDSN, nil
	}

	if cfg.MySQLHost == "" || cfg.MySQLPort == "" || cfg.MySQLUser == "" || cfg.MySQLDatabaseName == "" {
		return "", errors.New("MYSQL_DSN or MYSQL_HOST/MYSQL_PORT/MYSQL_USER/MYSQL_DATABASE_NAME is required when DATA_BACKEND=mysql")
	}

	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true",
		cfg.MySQLUser,
		cfg.MySQLPassword,
		cfg.MySQLHost,
		cfg.MySQLPort,
		cfg.MySQLDatabaseName,
	), nil
}

func applyMySQLPoolConfig(db *sql.DB, cfg MySQLPoolConfig) {
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = DefaultMySQLPoolConfig().MaxOpenConns
	}
	if cfg.MaxIdleConns < 0 {
		cfg.MaxIdleConns = 0
	}
	if cfg.ConnMaxLifetime <= 0 {
		cfg.ConnMaxLifetime = DefaultMySQLPoolConfig().ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime <= 0 {
		cfg.ConnMaxIdleTime = DefaultMySQLPoolConfig().ConnMaxIdleTime
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
}
