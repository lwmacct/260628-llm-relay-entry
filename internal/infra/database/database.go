package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
)

func Open(ctx context.Context, cfg config.ServerDatabase) (*bun.DB, error) {
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(BuildDSN(cfg))))
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return bun.NewDB(sqlDB, pgdialect.New()), nil
}

func BuildDSN(cfg config.ServerDatabase) string {
	user := configValue(cfg.User, "PGUSER", "postgres")
	password := configValue(cfg.Password, "PGPASSWORD", "")
	database := configValue(cfg.Database, "PGDATABASE", user)
	host := configValue(cfg.Host, "PGHOST", "localhost")
	port := configValue(cfg.Port, "PGPORT", "5432")
	dsn := url.URL{Scheme: "postgres", User: url.User(user), Path: "/" + database}
	if password != "" {
		dsn.User = url.UserPassword(user, password)
	}
	if strings.HasPrefix(host, "/") {
		dsn.Host = "localhost"
		query := dsn.Query()
		query.Set("host", host)
		dsn.RawQuery = query.Encode()
	} else {
		dsn.Host = net.JoinHostPort(host, port)
	}
	query := dsn.Query()
	query.Set("sslmode", "disable")
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func configValue(value, envKey, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "${"+envKey+"}" {
		if fromEnv := strings.TrimSpace(os.Getenv(envKey)); fromEnv != "" {
			return fromEnv
		}
		return fallback
	}
	return value
}
