// Package bus is the match-server's connection to the Redis event bus: durable
// Streams for commands (match.result / match.ended) and PubSub for ephemeral
// signals (presence). The match-server is a real-time UDP loop, so Publish is
// FIRE-AND-FORGET off the hot path — messages go to a buffered channel drained by
// a background goroutine, and a full buffer drops (with a log) rather than ever
// blocking the match tick. Envelopes are the shared bus.Envelope protobuf; the
// concrete event rides in Envelope.payload (see proto/bus/events.proto).
package bus

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	"libmadoka/match-server/bus/pb"
)

const (
	publishBuffer = 4096   // drained by one goroutine; full => drop (never block the tick)
	streamMaxLen  = 100000 // approx cap per stream so it can't grow unbounded
)

type outMsg struct {
	key string
	env []byte
}

// Bus wraps a Redis client with the envelope + routing conventions. One per
// process. Streams live at "stream:<type>", PubSub channels at "ps:<type>",
// presence keys at "presence:<accountID>".
type Bus struct {
	rdb    *redis.Client
	source string // stamped into Envelope.source (this layer: "match")
	node   string // this instance's id, written into presence keys
	out    chan outMsg
	ctx    context.Context
	cancel context.CancelFunc
}

// New connects to Redis (redisURL is a redis:// URL) and starts the publish
// drainer. It fails fast if Redis is unreachable at startup.
func New(redisURL, source, node string) (*Bus, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("bus: bad REDIS_URL %q: %w", redisURL, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bus{
		rdb:    redis.NewClient(opt),
		source: source,
		node:   node,
		out:    make(chan outMsg, publishBuffer),
		ctx:    ctx,
		cancel: cancel,
	}
	if err := b.rdb.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("bus: redis unreachable: %w", err)
	}
	go b.drain()
	return b, nil
}

func (b *Bus) wrap(typ string, msg proto.Message) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&pb.Envelope{
		Type:     typ,
		Source:   b.source,
		TsUnixMs: time.Now().UnixMilli(),
		Payload:  payload,
	})
}

// Publish enqueues a DURABLE event to stream:<typ>. NON-BLOCKING and safe from
// the match loop: a full buffer drops the message with a log instead of stalling
// the tick. Use for match.result / match.ended.
func (b *Bus) Publish(typ string, msg proto.Message) {
	env, err := b.wrap(typ, msg)
	if err != nil {
		log.Printf("[bus] marshal %s: %v", typ, err)
		return
	}
	select {
	case b.out <- outMsg{key: "stream:" + typ, env: env}:
	default:
		log.Printf("[bus] publish buffer full — dropped %s", typ)
	}
}

// PublishPS emits an EPHEMERAL event to ps:<typ> (best-effort). Also off the hot
// path via the same drainer.
func (b *Bus) PublishPS(typ string, msg proto.Message) {
	env, err := b.wrap(typ, msg)
	if err != nil {
		log.Printf("[bus] marshal %s: %v", typ, err)
		return
	}
	select {
	case b.out <- outMsg{key: "ps:" + typ, env: env}:
	default:
		log.Printf("[bus] publish buffer full — dropped %s", typ)
	}
}

func (b *Bus) drain() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case m := <-b.out:
			var err error
			if strings.HasPrefix(m.key, "stream:") {
				err = b.rdb.XAdd(b.ctx, &redis.XAddArgs{
					Stream: m.key, MaxLen: streamMaxLen, Approx: true,
					Values: map[string]interface{}{"env": m.env},
				}).Err()
			} else {
				err = b.rdb.Publish(b.ctx, m.key, m.env).Err()
			}
			if err != nil && b.ctx.Err() == nil {
				log.Printf("[bus] deliver %s: %v", m.key, err)
			}
		}
	}
}

// SubscribeStream joins <group> on stream:<typ> and runs handler for each
// message. The group is created if missing; each message is XACKed only on
// handler success — a returned error leaves it PENDING for redelivery, so
// consumers must be idempotent. Runs in its own goroutine.
func (b *Bus) SubscribeStream(typ, group, consumer string, handler func(*pb.Envelope) error) {
	stream := "stream:" + typ
	if err := b.rdb.XGroupCreateMkStream(b.ctx, stream, group, "0").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		log.Printf("[bus] group create %s/%s: %v", stream, group, err)
	}
	go func() {
		for b.ctx.Err() == nil {
			res, err := b.rdb.XReadGroup(b.ctx, &redis.XReadGroupArgs{
				Group: group, Consumer: consumer,
				Streams: []string{stream, ">"}, Count: 32, Block: 5 * time.Second,
			}).Result()
			if err != nil {
				if err == redis.Nil || b.ctx.Err() != nil {
					continue
				}
				log.Printf("[bus] XREADGROUP %s: %v", stream, err)
				time.Sleep(time.Second)
				continue
			}
			for _, s := range res {
				for _, msg := range s.Messages {
					raw, _ := msg.Values["env"].(string)
					var env pb.Envelope
					if err := proto.Unmarshal([]byte(raw), &env); err != nil {
						log.Printf("[bus] bad envelope on %s: %v — dropping", stream, err)
						b.rdb.XAck(b.ctx, stream, group, msg.ID)
						continue
					}
					if err := handler(&env); err != nil {
						log.Printf("[bus] handler %s: %v (left pending)", env.Type, err)
						continue
					}
					b.rdb.XAck(b.ctx, stream, group, msg.ID)
				}
			}
		}
	}()
}

// SubscribePS runs handler for each ephemeral message on ps:<typ>. Lossy by
// nature (PubSub); use for presence/notifications, never for money.
func (b *Bus) SubscribePS(typ string, handler func(*pb.Envelope)) {
	sub := b.rdb.Subscribe(b.ctx, "ps:"+typ)
	go func() {
		ch := sub.Channel()
		for {
			select {
			case <-b.ctx.Done():
				_ = sub.Close()
				return
			case m, ok := <-ch:
				if !ok {
					return
				}
				var env pb.Envelope
				if err := proto.Unmarshal([]byte(m.Payload), &env); err != nil {
					log.Printf("[bus] bad ps envelope on %s: %v", typ, err)
					continue
				}
				handler(&env)
			}
		}
	}()
}

// SetPresence writes presence:<accountID> = this node with a TTL so any layer can
// route a push to the owning instance. Refresh well within ttl.
func (b *Bus) SetPresence(accountID int64, ttl time.Duration) error {
	return b.rdb.Set(b.ctx, presenceKey(accountID), b.node, ttl).Err()
}

// GetNode returns the node currently owning accountID's connection, or "" (redis.Nil) if none.
func (b *Bus) GetNode(accountID int64) (string, error) {
	node, err := b.rdb.Get(b.ctx, presenceKey(accountID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return node, err
}

// Close stops the drainer and subscriptions and closes the Redis connection.
func (b *Bus) Close() error {
	b.cancel()
	return b.rdb.Close()
}

// Decode unmarshals an envelope's payload into msg.
func Decode(env *pb.Envelope, msg proto.Message) error {
	return proto.Unmarshal(env.Payload, msg)
}

func presenceKey(id int64) string { return fmt.Sprintf("presence:%d", id) }
