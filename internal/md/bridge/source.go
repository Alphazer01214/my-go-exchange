package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"my-go-exchange/internal/md"
)

// Config configures the Python CTP bridge subprocess.
type Config struct {
	Python   string // python executable; default "python"
	Script   string // path to md_bridge.py
	Front    string
	Broker   string
	User     string
	Password string
	Symbols  []string
	Logger   *slog.Logger
}

// Source runs the Python CTP bridge and parses NDJSON ticks from stdout.
type Source struct {
	cfg Config
}

func New(cfg Config) *Source {
	if cfg.Python == "" {
		cfg.Python = "python"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Source{cfg: cfg}
}

func (s *Source) Name() string { return "ctp-bridge" }

func (s *Source) Run(ctx context.Context, out chan<- md.Tick) error {
	if s.cfg.Script == "" {
		return errors.New("bridge script path is required")
	}
	if len(s.cfg.Symbols) == 0 {
		return errors.New("at least one symbol is required")
	}

	args := []string{
		s.cfg.Script,
		"--front", s.cfg.Front,
		"--symbols", strings.Join(s.cfg.Symbols, ","),
		"--broker", s.cfg.Broker,
	}
	if s.cfg.User != "" {
		args = append(args, "--user", s.cfg.User)
	}
	if s.cfg.Password != "" {
		args = append(args, "--password", s.cfg.Password)
	}

	cmd := exec.CommandContext(ctx, s.cfg.Python, args...)
	cmd.Env = os.Environ()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bridge: %w", err)
	}
	s.cfg.Logger.Info("bridge started",
		"pid", cmd.Process.Pid,
		"front", s.cfg.Front,
		"symbols", s.cfg.Symbols,
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.consume(ctx, stdout, out)
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return fmt.Errorf("bridge exited: %w", waitErr)
	}
	return nil
}

func (s *Source) consume(ctx context.Context, r io.Reader, out chan<- md.Tick) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			s.cfg.Logger.Warn("bridge bad json", "line", line)
			continue
		}
		switch envelope.Type {
		case "tick":
			var tick md.Tick
			if err := json.Unmarshal([]byte(line), &tick); err != nil {
				s.cfg.Logger.Warn("bridge bad tick", "err", err)
				continue
			}
			if tick.InstrumentID == "" {
				continue
			}
			select {
			case out <- tick:
			case <-ctx.Done():
				return
			}
		case "status":
			var st md.Status
			_ = json.Unmarshal([]byte(line), &st)
			s.cfg.Logger.Info("bridge status",
				"event", st.Event,
				"instrument", st.InstrumentID,
				"trading_day", st.TradingDay,
				"error_id", st.ErrorID,
				"error_msg", st.ErrorMsg,
				"reason", st.Reason,
			)
		default:
			s.cfg.Logger.Debug("bridge line", "line", line)
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
		s.cfg.Logger.Warn("bridge stdout closed", "err", err)
	}
}
