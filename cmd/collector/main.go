package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/senton89/mexc-ticker/internal/agg"
	"github.com/senton89/mexc-ticker/internal/api"
	"github.com/senton89/mexc-ticker/internal/config"
	"github.com/senton89/mexc-ticker/internal/model"
	"github.com/senton89/mexc-ticker/internal/source"
	"github.com/senton89/mexc-ticker/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run собирает пайплайн source → agg → store и ждёт сигнала.
// Вынесен из main, чтобы defer'ы отработали до os.Exit.
func run() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL, cfg.BatchSize, cfg.FlushInterval, log)
	if err != nil {
		return err
	}
	defer st.Close()

	src := source.New(cfg.MEXCBaseURL, cfg.Symbols, cfg.PollInterval, log)
	ag := agg.New(5*time.Second, log)

	trades := make(chan model.Trade, 1024)
	candles := make(chan model.Candle, 256)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return src.Run(gctx, trades) })
	g.Go(func() error { return ag.Run(gctx, trades, candles) })
	g.Go(func() error { return st.Run(gctx, candles) })

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(st, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	g.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(gctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	})

	log.Info("collector started", "symbols", cfg.Symbols, "poll", cfg.PollInterval)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("collector stopped")
	return nil
}
