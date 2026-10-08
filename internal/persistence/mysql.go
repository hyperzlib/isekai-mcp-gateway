package persistence

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"net"
	"os"
	"strconv"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/security"
)

type Store struct {
	DB    *gorm.DB
	SQL   *sql.DB
	Vault *security.Vault
}

func Open(c *config.Config) (*Store, error) {
	s := c.Static.Storage
	// 组装 MySQL DSN：统一 UTC 时区，限定连接与读写超时，字符集为 utf8mb4。
	d := driver.NewConfig()
	d.User, d.Passwd, d.Net = s.Username, c.Secrets.DBPassword, "tcp"
	d.Addr, d.DBName = net.JoinHostPort(s.Host, strconv.Itoa(s.Port)), s.Database
	d.ParseTime, d.Loc = true, time.UTC
	d.Timeout, d.ReadTimeout, d.WriteTimeout = 5*time.Second, 10*time.Second, 10*time.Second
	d.Params = map[string]string{"charset": "utf8mb4"}
	// 启用 TLS 时至少 TLS 1.2，校验证书链并允许指定 ServerName。
	if s.TLS.Enabled {
		t := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.TLS.ServerName}
		if t.ServerName == "" {
			t.ServerName = s.Host
		}
		if s.TLS.CAPath != "" {
			b, err := os.ReadFile(s.TLS.CAPath)
			if err != nil {
				return nil, err
			}
			t.RootCAs = x509.NewCertPool()
			if !t.RootCAs.AppendCertsFromPEM(b) {
				return nil, errors.New("invalid database CA")
			}
		}
		if err := driver.RegisterTLSConfig("gateway", t); err != nil {
			return nil, err
		}
		d.TLSConfig = "gateway"
	}
	db, err := gorm.Open(mysql.Open(d.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, failure.Storage("database.connect", err)
	}
	conn, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 连接池上限与配置一致，连接寿命限为 5 分钟以避开服务端回收。
	conn.SetMaxOpenConns(s.PoolSize)
	conn.SetMaxIdleConns(s.PoolSize)
	conn.SetConnMaxLifetime(5 * time.Minute)
	v, err := security.NewVault(c.Secrets.Storage)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Store{DB: db, SQL: conn, Vault: v}, nil
}

func (s *Store) Health(ctx context.Context) error { return s.SQL.PingContext(ctx) }
