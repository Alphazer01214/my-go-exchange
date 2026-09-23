package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"

	"my-go-exchange/internal/market"
	"my-go-exchange/internal/md"
	"my-go-exchange/internal/md/bridge"
	"my-go-exchange/internal/md/natspub"
	"my-go-exchange/internal/md/natsserver"
	"my-go-exchange/internal/web"
)

type appConfig struct {
	Listen string `mapstructure:"listen"`
	Bridge struct {
		Python   string   `mapstructure:"python"`
		Script   string   `mapstructure:"script"`
		Front    string   `mapstructure:"front"`
		Broker   string   `mapstructure:"broker"`
		User     string   `mapstructure:"user"`
		Password string   `mapstructure:"password"`
		Symbols  []string `mapstructure:"symbols"`
	} `mapstructure:"bridge"`
	NATS struct {
		Embedded bool   `mapstructure:"embedded"`
		URL      string `mapstructure:"url"`
		Port     int    `mapstructure:"port"`
		StoreDir string `mapstructure:"store_dir"`
	} `mapstructure:"nats"`
}

func loadConfig() (appConfig, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("config")
	v.AddConfigPath(".")
	v.SetDefault("listen", "127.0.0.1:8088")
	v.SetDefault("bridge.python", "python")
	v.SetDefault("bridge.script", "tools/md_bridge/md_bridge.py")
	v.SetDefault("bridge.front", "tcp://182.254.243.31:40011")
	v.SetDefault("bridge.broker", "9999")
	v.SetDefault("bridge.symbols", []string{"cu2611", "al2611", "rb2701", "au2612"})
	v.SetDefault("nats.embedded", true)
	v.SetDefault("nats.port", 4222)
	v.SetDefault("nats.store_dir", "data/nats")
	v.SetDefault("nats.url", "nats://127.0.0.1:4222")

	var cfg appConfig
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			// use defaults
		} else {
			return cfg, fmt.Errorf("read config: %w", err)
		}
	}
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := loadConfig()
	if err != nil {
		log.Error("config error", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var embedded *natsserver.Embedded
	natsURL := cfg.NATS.URL
	if cfg.NATS.Embedded {
		storeDir := cfg.NATS.StoreDir
		if !filepath.IsAbs(storeDir) {
			if abs, absErr := filepath.Abs(storeDir); absErr == nil {
				storeDir = abs
			}
		}
		if err := os.MkdirAll(storeDir, 0o755); err != nil {
			log.Error("mkdir nats store", "err", err)
			os.Exit(1)
		}
		port := cfg.NATS.Port
		if port == 0 {
			port = -1
		}
		embedded, err = natsserver.Start(natsserver.Options{
			Port:     port,
			StoreDir: storeDir,
			Logger:   log,
		})
		if err != nil {
			log.Error("start embedded nats", "err", err)
			os.Exit(1)
		}
		defer embedded.Shutdown()
		natsURL = embedded.ClientURL()
	}

	nc, err := nats.Connect(natsURL, nats.Name("md-ingest"))
	if err != nil {
		log.Error("connect nats", "url", natsURL, "err", err)
		os.Exit(1)
	}
	defer nc.Close()

	pub, err := natspub.New(nc, log)
	if err != nil {
		log.Error("init jetstream publisher", "err", err)
		os.Exit(1)
	}

	hub := market.NewHub()
	ticks := make(chan md.Tick, 1024)
	src := bridge.New(bridge.Config{
		Python:   cfg.Bridge.Python,
		Script:   cfg.Bridge.Script,
		Front:    cfg.Bridge.Front,
		Broker:   cfg.Bridge.Broker,
		User:     cfg.Bridge.User,
		Password: cfg.Bridge.Password,
		Symbols:  cfg.Bridge.Symbols,
		Logger:   log,
	})

	pipelineDone := make(chan struct{})
	go func() {
		defer close(pipelineDone)
		for {
			select {
			case <-ctx.Done():
				return
			case t, ok := <-ticks:
				if !ok {
					return
				}
				hub.OnTick(t)
				if err := pub.Publish(ctx, t); err != nil {
					log.Warn("publish tick failed", "instrument", t.InstrumentID, "err", err)
				}
			}
		}
	}()

	srcDone := make(chan error, 1)
	go func() {
		srcDone <- src.Run(ctx, ticks)
	}()

	srv := web.NewServer(hub, web.Meta{
		Source:    src.Name(),
		Publisher: pub.Name(),
		NATS:      natsURL,
		Front:     cfg.Bridge.Front,
		Symbols:   cfg.Bridge.Symbols,
		Stream:    natspub.StreamName,
		Subject:   natspub.SubjectPrefix + ".>",
	}, log)

	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler()}
	httpErr := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", cfg.Listen, "ui", "http://"+cfg.Listen+"/")
		httpErr <- httpSrv.ListenAndServe()
	}()

	log.Info("md-ingest up",
		"front", cfg.Bridge.Front,
		"symbols", cfg.Bridge.Symbols,
		"nats", natsURL,
	)

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-srcDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("source failed", "err", err)
			cancel()
		}
	case err := <-httpErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http failed", "err", err)
			cancel()
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	cancel()
	<-pipelineDone
}
