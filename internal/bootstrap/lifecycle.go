package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"isekai-mcp-gateway/internal/auth/provider/zitadel"
	repository "isekai-mcp-gateway/internal/auth/repository/mysql"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/runtimefiles"
	"isekai-mcp-gateway/internal/wiki"
)

func Serve(c *config.Config, files *runtimefiles.Files, watchTemplates bool) (err error) {
	// defer 统一为启动错误附加 startup.serve 操作上下文
	defer func() {
		if err != nil {
			err = failure.Wrap("startup.serve", err)
		}
	}()

	// 建立可动态调整级别的 JSON 日志并设为全局 logger
	level := new(slog.LevelVar)
	outbound.SetLogLevel(level, c.App.Logging.Level)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if strings.HasPrefix(c.App.MediaWiki.APIURL, "http://") {
		slog.Warn("Wiki API uses HTTP; transport is not encrypted")
	}

	// 监听中断/终止信号，取消后触发优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 按需开启模板热加载
	if watchTemplates {
		if err := files.Templates.Watch(ctx); err != nil {
			return err
		}
		defer files.Templates.Close()
	}

	// 打开数据库并确认 schema 已迁移到当前版本
	store, err := persistence.Open(c)
	if err != nil {
		return err
	}
	defer store.SQL.Close()
	if err = store.CheckSchema(); err != nil {
		return err
	}

	// 限时构建出站依赖并发布，供后台 watcher 后续刷新
	manager := new(outbound.Manager)
	startup, cancel := context.WithTimeout(ctx, 45*time.Second)
	bundle, err := outbound.Build(startup, c)
	cancel()
	if err != nil {
		return err
	}
	manager.Publish(bundle)

	// 初始化认证服务、种子客户端与业务服务
	a, err := zitadel.New(c, repository.New(store), manager)
	if err != nil {
		return err
	}
	if err = a.SeedClients(ctx); err != nil {
		return failure.Wrap("startup.seed_clients", err)
	}
	business := &wiki.Service{Auth: a.Service, Repo: &wiki.Repository{Store: store}, Text: files.Templates}
	if err = business.Recover(ctx); err != nil {
		return failure.Wrap("startup.wiki_recovery", err)
	}
	if err = business.PrepareRuntime(ctx); err != nil {
		return failure.Wrap("startup.uploads", err)
	}

	// 组装路由并监听配置的 TCP 端口
	router, err := assemble(c, a, files, business)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(c.Static.Server.Host, strconv.Itoa(c.Static.Server.Port)))
	if err != nil {
		return err
	}

	// 配置 HTTP 超时与 TLS 最低版本
	server := &http.Server{Handler: router.Handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32768,
		BaseContext: func(net.Listener) context.Context { return ctx },
		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
	}

	// 启动清理与配置监听三个后台任务，退出前统一等待
	var background sync.WaitGroup
	background.Add(3)
	go func() { defer background.Done(); business.RunCleanup(ctx) }()
	go func() { defer background.Done(); manager.Watch(ctx, c, level) }()
	go func() { defer background.Done(); a.RunCleanup(ctx) }()

	// 依据 TLS 配置选择监听方式
	done := make(chan error, 1)
	go func() {
		settings := c.Static.Server.TLS
		if settings.Enabled {
			done <- server.ServeTLS(listener, settings.CertFile, settings.KeyFile)
			return
		}
		done <- server.Serve(listener)
	}()
	slog.Info("gateway listening", "address", listener.Addr().String())

	// 等待服务报错或收到关闭信号
	select {
	case err = <-done:
		stop()
	case <-ctx.Done():
	}

	// 限时优雅关闭，超时则强制关闭
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		failure.Log(shutdown, "server.shutdown", failure.Wrap("server.shutdown", shutdownErr))
		if closeErr := server.Close(); closeErr != nil {
			failure.Log(shutdown, "server.close", failure.Wrap("server.close", closeErr))
		}
	}
	background.Wait()

	// http.ErrServerClosed 属于正常关闭，不作为错误返回
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
