package main

import (
	"context"
	"ezbkTools/internal/adapters/ezbookkeeping"
	"ezbkTools/internal/adapters/httpapi"
	"ezbkTools/internal/adapters/sqlite"
	"ezbkTools/internal/app"
	"ezbkTools/internal/config"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lmittmann/tint"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

func main() {
	cfg := config.NewConfig()
	gin.SetMode(cfg.GinMode)
	logger, cleanup, err := initLogger(cfg.LogLevel == "debug")
	if err != nil {
		log.Fatalf("init logger error: %v", err)
		return
	}

	db, err := sqlite.Open(cfg.DBDatabase)
	if err != nil {
		logger.Error("open db error", "err", err)
		return
	}

	jobs := sqlite.NewJob(db)
	categories := sqlite.NewCategory(db)
	accounts := sqlite.NewAccount(db)

	ezClient := ezbookkeeping.NewEzBookkeepingClient(cfg.EzBookKeepingKey, cfg.EzBookKeepingBaseURL)

	syncService := app.NewSyncService(jobs, ezClient, categories, accounts)
	worker := app.NewWorker(jobs, syncService)

	// syncService — він же JobWaker: після нового мапінгу будить воркер
	mappingService := app.NewMappingService(categories, accounts, ezClient, jobs, syncService)

	srv := httpapi.NewServer(syncService, mappingService, httpapi.AuthConfig{
		Login:        cfg.AuthLogin,
		PasswordHash: cfg.AuthPasswordHash,
		Secret:       cfg.AuthSecret,
	})

	httpSrv := &http.Server{
		Addr:    cfg.BindAddr,
		Handler: srv,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(ctx)
	}()

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("Server starting...")
		srvErr <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-srvErr:
		logger.Error("server initialization error", "err", err)
	case <-ctx.Done():
		var errs []error

		shutdownCtx, cancelShutdown := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)

		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "err", err)
			errs = append(errs, err)

			if closeErr := httpSrv.Close(); closeErr != nil {
				errs = append(errs, closeErr)
			}
		}

		cancelShutdown()

		workerCtx, cancelWorker := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)

		select {
		case <-workerDone:
			logger.Info("Worker stopped gracefully")

		case <-workerCtx.Done():
			logger.Warn("Worker shutdown timeout")
			errs = append(errs, workerCtx.Err())
		}

		cancelWorker()

		if err := db.Close(); err != nil {
			errs = append(errs, err)
		}

		logger.Info("server stopped")
		logger.Info("server stopped", "errors", errs)
		cleanup() // скидання буфера логера
	}

}

func initLogger(dev bool) (*slog.Logger, func(), error) {
	if dev {
		logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: time.TimeOnly,
			AddSource:  true,
		}))
		slog.SetDefault(logger)
		return logger, func() {}, nil
	}

	zapLogger, err := zap.NewProduction()
	if err != nil {
		return nil, nil, err
	}

	logger := slog.New(zapslog.NewHandler(zapLogger.Core(), zapslog.WithCaller(true)))
	slog.SetDefault(logger)

	cleanup := func() {
		_ = zapLogger.Sync()
	}

	return logger, cleanup, nil
}
