package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sim7600d/internal/api"
	"sim7600d/internal/atexec"
	"sim7600d/internal/callaudio"
	"sim7600d/internal/modem"
	"sim7600d/internal/reconciler"
	"sim7600d/internal/store"
	"sim7600d/internal/ttyx"
	"sim7600d/internal/urc"
)

var Version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "sim7600d: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := Parse(args)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: parseLevel(cfg.Log.Level)}))
	slog.SetDefault(logger)
	slog.Info("starting", "version", Version, "bind", cfg.Server.Bind, "tty", cfg.Modem.TTY, "db", cfg.Storage.Path)

	tr, err := ttyx.OpenTTY(cfg.Modem.TTY)
	if err != nil {
		return fmt.Errorf("open tty: %w", err)
	}
	bus := urc.NewBus()
	ex := atexec.New(tr, bus)
	defer ex.Close()
	defer bus.Close()

	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	mo, err := modem.New(ex, bus, st)
	if err != nil {
		return err
	}
	defer mo.Close()

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := mo.Boot(bootCtx); err != nil {
		bootCancel()
		return fmt.Errorf("modem boot: %w", err)
	}
	bootCancel()

	rec := reconciler.New(ex, st, mo)
	if err := rec.Boot(context.Background()); err != nil {
		slog.Warn("reconciler boot warned", "err", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go rec.RunForever(ctx, bus)
	go retentionSweepForever(ctx, st, cfg.Retention)

	var audioBridge *callaudio.Bridge
	if cfg.Audio.Device != "" {
		audioBridge = callaudio.New(mo, cfg.Audio.Device)
		defer audioBridge.Close()
		slog.Info("call audio enabled", "device", cfg.Audio.Device, "sample_rate", callaudio.SampleRate)
	}

	router := api.NewRouter(api.Config{
		AuthToken: cfg.authToken,
		Modem:     mo,
		Store:     st,
		CallAudio: audioBridge,
		Admin: &api.Admin{
			Reconcile: rec.Reconcile,
			QueueInfo: func() map[string]any {
				queued, capacity, active, transportUp := ex.QueueStats()
				return map[string]any{
					"version": Version, "queued_requests": queued,
					"queue_capacity": capacity, "command_active": active,
					"transport_up": transportUp,
				}
			},
			Exec: ex,
		},
		AllowATPassthrough: cfg.Admin.ATPassthrough,
		AllowModemReset:    cfg.Admin.AllowModemReset,
	})
	srv := newHTTPServer(cfg.Server.Bind, router)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "err", err)
		}
	}()
	slog.Info("listening", "bind", cfg.Server.Bind)

	select {
	case <-ctx.Done():
	case <-ex.Done():
		return fmt.Errorf("modem transport closed")
	}
	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("http shutdown", "err", err)
		// Cancel connections whose handlers did not finish inside the grace
		// period so deferred modem/database teardown stays bounded.
		if closeErr := srv.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			slog.Warn("http force close", "err", closeErr)
		}
	}
	return nil
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		// ReadTimeout and WriteTimeout must remain unset because their socket
		// deadlines survive WebSocket hijacking. Request bodies are bounded by
		// middleware; event streams and call audio are intentionally long lived.
	}
}

func retentionSweepForever(ctx context.Context, st *store.Store, cfg RetentionConfig) {
	if cfg.IdemHours <= 0 {
		cfg.IdemHours = 24
	}
	sweep := func() {
		now := time.Now().UTC()
		policy := store.RetentionPolicy{
			EventsBefore: cutoffDays(now, cfg.EventsDays),
			SMSBefore:    cutoffDays(now, cfg.SMSDays),
			CallsBefore:  cutoffDays(now, cfg.CallsDays),
			PartsBefore:  now.Add(-7 * 24 * time.Hour),
		}
		c, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		result, err := st.SweepRetention(c, policy)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("retention sweep", "err", err)
			return
		}
		idem, err := st.SweepIdem(c, now.Add(-time.Duration(cfg.IdemHours)*time.Hour))
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("idempotency sweep", "err", err)
			return
		}
		if result.Total()+idem > 0 {
			slog.Info("retention sweep complete", "events", result.Events, "sms", result.SMS,
				"calls", result.Calls, "parts", result.Parts, "idempotency", idem)
		}
	}
	sweep()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

func cutoffDays(now time.Time, days int) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}
