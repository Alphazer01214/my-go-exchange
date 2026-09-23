package natspub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"my-go-exchange/internal/md"
)

// StreamName is the JetStream stream holding market ticks.
const StreamName = "MD_TICKS"

// SubjectPrefix is the per-instrument subject prefix: md.tick.<instrument>
const SubjectPrefix = "md.tick"

// Publisher writes ticks to NATS JetStream.
type Publisher struct {
	nc  *nats.Conn
	js  nats.JetStreamContext
	log *slog.Logger
}

func New(nc *nats.Conn, log *slog.Logger) (*Publisher, error) {
	if log == nil {
		log = slog.Default()
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}
	p := &Publisher{nc: nc, js: js, log: log}
	if err := p.ensureStream(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Publisher) Name() string { return "nats-jetstream" }

func (p *Publisher) ensureStream() error {
	cfg := &nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{SubjectPrefix + ".>"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    24 * time.Hour,
		Replicas:  1,
	}
	info, err := p.js.StreamInfo(StreamName)
	if err == nil && info != nil {
		_, err = p.js.UpdateStream(cfg)
		return err
	}
	_, err = p.js.AddStream(cfg)
	return err
}

func (p *Publisher) Publish(ctx context.Context, tick md.Tick) error {
	if tick.InstrumentID == "" {
		return fmt.Errorf("empty instrument id")
	}
	buf, err := json.Marshal(tick)
	if err != nil {
		return err
	}
	subject := SubjectPrefix + "." + tick.InstrumentID
	_, err = p.js.Publish(subject, buf, nats.Context(ctx))
	return err
}

func (p *Publisher) Close() error {
	// connection lifecycle is owned by the caller
	return nil
}
