package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jason-i-magno/career/internal/web"
)

// runServe serves the dashboard and review session over HTTP, for use from a
// phone. It listens on loopback by default because the server has no login:
// the intended way in from another device is a proxy that adds TLS and access
// control, such as `tailscale serve`.
func runServe(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "serve")
	addr := fs.String("addr", "127.0.0.1:8080", "address to listen on")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	// Under systemd the journal stamps every line with its own time, and there
	// is no terminal to press ctrl-c in.
	journal := toJournal(env.Err)

	log := serveLogger(env.Err, journal)
	h, err := web.New(env.Store, env.Now, log)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listening: %w", err)
	}
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok && !tcp.IP.IsLoopback() {
		fmt.Fprintf(env.Err, "%s %s is reachable from other machines, and there is no login\n",
			yellow("warning:"), ln.Addr())
	}

	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	hint := " " + dim("(ctrl-c to stop)")
	if journal {
		hint = ""
	}
	fmt.Fprintf(env.Out, "%s http://%s%s\n", green("serving"), ln.Addr(), hint)

	select {
	case err := <-errc:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}

	// Let an in-flight grade finish writing rather than cutting it off.
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutting down: %w", err)
	}
	return nil
}

// serveLogger returns the request logger. Output bound for the journal leaves
// out the time, which the journal records for every line itself.
func serveLogger(w io.Writer, journal bool) *slog.Logger {
	opts := &slog.HandlerOptions{}
	if journal {
		opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}
			return a
		}
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
