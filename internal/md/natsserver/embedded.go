package natsserver

import (
	"fmt"
	"log/slog"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// Embedded runs an in-process NATS JetStream server.
type Embedded struct {
	srv *natsserver.Server
}

// Options for the embedded server.
type Options struct {
	Port     int    // 0 = random free port
	StoreDir string // JetStream file store directory
	Logger   *slog.Logger
}

func Start(opts Options) (*Embedded, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.StoreDir == "" {
		return nil, fmt.Errorf("store dir is required")
	}
	nsOpts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      opts.Port,
		JetStream: true,
		StoreDir:  opts.StoreDir,
		NoLog:     true,
		NoSigs:    true,
	}
	srv, err := natsserver.NewServer(nsOpts)
	if err != nil {
		if opts.Port > 0 {
			nsOpts.Port = -1
			srv, err = natsserver.NewServer(nsOpts)
		}
		if err != nil {
			return nil, fmt.Errorf("new nats server: %w", err)
		}
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		return nil, fmt.Errorf("nats server not ready on %s", nsOpts.Host)
	}
	if !srv.JetStreamEnabled() {
		srv.Shutdown()
		return nil, fmt.Errorf("jetstream not enabled")
	}
	opts.Logger.Info("embedded nats jetstream ready", "addr", srv.ClientURL(), "store", opts.StoreDir)
	return &Embedded{srv: srv}, nil
}

func (e *Embedded) ClientURL() string {
	return e.srv.ClientURL()
}

func (e *Embedded) Shutdown() {
	if e.srv != nil {
		e.srv.Shutdown()
		e.srv.WaitForShutdown()
	}
}
