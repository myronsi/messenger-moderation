package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/myronsi/messenger-moderation/console"
	"github.com/myronsi/messenger-moderation/internal/appdb"
	"github.com/myronsi/messenger-moderation/internal/cases"
	"github.com/myronsi/messenger-moderation/internal/config"
	"github.com/myronsi/messenger-moderation/internal/content"
	"github.com/myronsi/messenger-moderation/internal/db"
	"github.com/myronsi/messenger-moderation/internal/decisions"
	"github.com/myronsi/messenger-moderation/internal/httpserver"
	"github.com/myronsi/messenger-moderation/internal/messenger"
	"github.com/myronsi/messenger-moderation/internal/notices"
	"github.com/myronsi/messenger-moderation/internal/review"
	"github.com/myronsi/messenger-moderation/internal/retention"
	"github.com/myronsi/messenger-moderation/internal/staff"
	"github.com/myronsi/messenger-moderation/internal/stats"
	"github.com/myronsi/messenger-moderation/internal/status"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "moderation:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	command := "serve"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	if command == "healthcheck" {
		return healthcheck()
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	switch command {
	case "serve":
		return serve(cfg, log)
	case "migrate":
		return db.Migrate(cfg.DatabaseURL)
	case "create-staff":
		return createStaff(cfg, args, stdin, stdout)
	case "stats-rerun":
		return statsRerun(cfg, log, args, stdout)
	default:
		return fmt.Errorf("unknown command %q: use serve, migrate, create-staff or stats-rerun", command)
	}
}

func serve(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	staffSvc, err := staff.NewService(pool, cfg.Key(), cfg.SessionTTL)
	if err != nil {
		return err
	}
	consoleFS, err := consoleFiles(cfg.ConsoleDir)
	if err != nil {
		return err
	}
	var reviewStore *review.Store
	var appPool *pgxpool.Pool
	if cfg.Messenger.ReadDatabaseURL != "" {
		appPool, err = appdb.Open(ctx, cfg.Messenger.ReadDatabaseURL)
		if err != nil {
			return err
		}
		defer appPool.Close()
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := appPool.Ping(pingCtx); err != nil {
			log.ErrorContext(ctx, "messenger database is not reachable; queue, account pages and search fail until it is", "error", err)
		}
		cancel()
		reviewStore = review.NewStore(appPool)
	} else {
		log.Warn("MESSENGER_READ_DATABASE_URL is not set; queue, account pages and search answer 503")
	}
	caseStore := cases.NewStore(pool, appPool, log)
	messengerClient := messenger.New(cfg.Messenger.InternalURL, cfg.Messenger.InternalToken, nil)
	decisionSvc := decisions.NewService(pool, appPool, messengerClient, caseStore, log)
	contentSvc := content.NewService(pool, appPool, messengerClient, log)
	reviewSvc := review.NewService(reviewStore, pool, decisionSvc.Reviews)
	noticeSvc := notices.NewService(pool, appPool, noticeMailer(cfg), log)
	if !noticeSvc.MailEnabled() {
		log.Warn("SMTP_HOST or SMTP_FROM is not set; decisions on notices are not emailed to the notifier")
	}
	if cfg.Messenger.InternalURL == "" {
		log.Warn("MESSENGER_INTERNAL_URL is not set; decisions answer 503")
	}

	reporter := status.NewReporter(pool)
	monitor := status.NewMonitor(pool, status.Options{
		Probes: statusProbes(cfg, pool.Ping), Notifier: alertNotifier(cfg), Log: log, Interval: cfg.Status.Interval,
	})
	messengerDB, err := openMessenger(ctx, cfg)
	if err != nil {
		return err
	}
	if messengerDB != nil {
		defer messengerDB.Close()
	} else {
		log.Warn("MESSENGER_READ_DATABASE_URL is not set: no daily statistics")
	}
	statsStore := stats.NewStore(pool)
	collector := newCollector(messengerDB, statsStore)
	statsHandlers := &stats.Handlers{Store: statsStore, Collector: collector, Log: log}
	handler := httpserver.New(httpserver.Deps{
		Staff: staffSvc, PublicOrigin: cfg.PublicOrigin, TrustProxy: cfg.TrustProxy, Environment: cfg.Environment, Version: version,
		Console: consoleFS, Ready: pool.Ping, Log: log,
		Register: func(mux *http.ServeMux, s *httpserver.Server) {
			status.Register(mux, s, reporter, cfg.Status.PublicOrigins)
			review.Register(mux, s, reviewSvc, log)
			decisions.Register(mux, s, decisionSvc, log)
			content.Register(mux, s, contentSvc, log)
			notices.Register(mux, s, noticeSvc, log)
			statsHandlers.Register(mux, s)
		},
	})
	go monitor.Run(ctx)
	go noticeSvc.RunMail(ctx, time.Minute)
	if appPool != nil {
		go caseStore.Run(ctx, time.Minute)
	}
	srv := &http.Server{
		Addr: cfg.Addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	go purgeSessions(ctx, log, staffSvc)
	go retention.Run(ctx, pool, log, retention.RunEvery)
	if collector != nil {
		job := &stats.Job{Collector: collector, Store: statsStore, Log: log}
		go job.Run(ctx, stats.RunEvery)
	}
	errs := make(chan error, 1)
	go func() {
		log.Info("moderation listening", "addr", cfg.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
		close(errs)
	}()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func purgeSessions(ctx context.Context, log *slog.Logger, svc *staff.Service) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := svc.PurgeExpiredSessions(ctx); err != nil {
				log.WarnContext(ctx, "purge staff sessions", "error", err)
			}
		}
	}
}

func openMessenger(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	if cfg.Messenger.ReadDatabaseURL == "" {
		return nil, nil
	}
	pool, err := db.Open(ctx, cfg.Messenger.ReadDatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("messenger database: %w", err)
	}
	return pool, nil
}

func newCollector(messengerDB *pgxpool.Pool, store *stats.Store) *stats.Collector {
	if messengerDB == nil {
		return nil
	}
	return &stats.Collector{Messenger: messengerDB, Moderation: store}
}

func statsRerun(cfg config.Config, log *slog.Logger, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("stats-rerun", flag.ContinueOnError)
	fromFlag := flags.String("from", "", "first day (YYYY-MM-DD)")
	toFlag := flags.String("to", "", "last day (YYYY-MM-DD), default: the same as -from")
	if err := flags.Parse(args); err != nil {
		return err
	}
	from, err := time.Parse(time.DateOnly, *fromFlag)
	if err != nil {
		return errors.New("usage: stats-rerun -from YYYY-MM-DD [-to YYYY-MM-DD]")
	}
	to := from
	if *toFlag != "" {
		if to, err = time.Parse(time.DateOnly, *toFlag); err != nil {
			return fmt.Errorf("to: %w", err)
		}
	}
	if to.Before(from) || !to.Before(stats.StartOfDay(time.Now())) {
		return errors.New("stats-rerun: -to must be on or after -from and before today")
	}
	ctx := context.Background()
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	messengerDB, err := openMessenger(ctx, cfg)
	if err != nil {
		return err
	}
	if messengerDB == nil {
		return errors.New("stats-rerun needs MESSENGER_READ_DATABASE_URL")
	}
	defer messengerDB.Close()
	store := stats.NewStore(pool)
	job := &stats.Job{Collector: newCollector(messengerDB, store), Store: store, Log: log}
	n, err := job.Rerun(ctx, from, to)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Recomputed %d days (%s to %s).\n", n, from.Format(time.DateOnly), to.Format(time.DateOnly))
	return err
}

func consoleFiles(dir string) (fs.FS, error) {
	if dir != "" {
		return os.DirFS(dir), nil
	}
	return fs.Sub(console.Files, "dist")
}

func createStaff(cfg config.Config, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("create-staff", flag.ContinueOnError)
	role := flags.String("role", "moderator", "admin or moderator")
	signed := flags.String("confidentiality-signed", "", "date the confidentiality promise was signed (YYYY-MM-DD); required for moderators")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: create-staff [-role admin|moderator] [-confidentiality-signed YYYY-MM-DD] <username>")
	}
	var signedAt *time.Time
	if *signed != "" {
		t, err := time.Parse(time.DateOnly, *signed)
		if err != nil {
			return fmt.Errorf("confidentiality-signed: %w", err)
		}
		signedAt = &t
	}
	password := os.Getenv("STAFF_PASSWORD")
	if password == "" {
		_, _ = fmt.Fprint(stdout, "Password (at least 12 characters): ")
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		password = strings.TrimRight(line, "\r\n")
	}

	ctx := context.Background()
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc, err := staff.NewService(pool, cfg.Key(), cfg.SessionTTL)
	if err != nil {
		return err
	}
	created, err := svc.Create(ctx, staff.NewStaff{Username: flags.Arg(0), Password: password, Role: staff.Role(*role), ConfidentialityOn: signedAt})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "\nCreated %s (%s).\nAdd this to an authenticator app now; it is not shown again.\nSecret: %s\nURI: %s\n",
		created.Staff.Username, created.Staff.Role, created.TOTPSecret, created.TOTPURI)
	return err
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

const healthURL = "http://127.0.0.1:8090/healthz"

func healthcheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz answered %d", resp.StatusCode)
	}
	return nil
}

func noticeMailer(cfg config.Config) notices.Mailer {
	if cfg.Mail.Host == "" || cfg.Mail.From == "" {
		return nil
	}
	return notices.NewSMTP(cfg.Mail.Host, cfg.Mail.Port, cfg.Mail.Username, cfg.Mail.Password, cfg.Mail.From)
}
