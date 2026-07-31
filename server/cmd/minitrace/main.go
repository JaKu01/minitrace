package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JaKu01/minitrace/server"
)

func main() {
	address := flag.String("address", ":8080", "HTTP listen address")
	database := flag.String("database", "minitrace.db", "SQLite database path")
	retention := flag.Duration("retention", server.MaxRetention, "trace retention (maximum 24h)")
	appDirectory := flag.String("app-dir", "", "optional Angular build directory")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	store, err := server.OpenStore(*database, *retention)
	if err != nil {
		logger.Error("open store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	var app fs.FS = server.EmbeddedApp()
	if *appDirectory != "" {
		app = os.DirFS(*appDirectory)
	}
	traceServer := server.New(store, logger, app)

	cleanupStop := make(chan struct{})
	go traceServer.StartRetentionCleanup(cleanupStop)

	httpServer := &http.Server{
		Addr:              *address,
		Handler:           traceServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("minitrace listening", "address", *address, "retention", store.Retention())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	close(cleanupStop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}
