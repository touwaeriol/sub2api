package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ============================================================
// RedisProxy implementation
// ============================================================

func (s *SDKServer) Get(ctx context.Context, req *pluginsdk.RedisKeyRequest) (*pluginsdk.RedisValueResponse, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	val, err := s.redis.Get(ctx, pfx.PrefixKey(req.GetKey())).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return &pluginsdk.RedisValueResponse{Exists: false}, nil
		}
		return nil, toGRPCError("plugin redis get", err)
	}
	return &pluginsdk.RedisValueResponse{Exists: true, Value: val}, nil
}

func (s *SDKServer) Set(ctx context.Context, req *pluginsdk.RedisSetRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.redis.Set(ctx, pfx.PrefixKey(req.GetKey()), req.GetValue(), 0).Err(); err != nil {
		return nil, toGRPCError("plugin redis set", err)
	}
	return &emptypb.Empty{}, nil
}

const (
	minTTLSeconds = 1
	maxTTLSeconds = 30 * 24 * 3600 // 30 days
)

func (s *SDKServer) SetEx(ctx context.Context, req *pluginsdk.RedisSetExRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	ttlSec := req.GetTtlSeconds()
	if ttlSec < minTTLSeconds || ttlSec > maxTTLSeconds {
		return nil, status.Errorf(codes.InvalidArgument, "ttl_seconds must be in [%d, %d]", minTTLSeconds, maxTTLSeconds)
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(ttlSec) * time.Second
	if err := s.redis.Set(ctx, pfx.PrefixKey(req.GetKey()), req.GetValue(), ttl).Err(); err != nil {
		return nil, toGRPCError("plugin redis setex", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) Del(ctx context.Context, req *pluginsdk.RedisDelRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	keys := req.GetKeys()
	if len(keys) == 0 {
		return &emptypb.Empty{}, nil
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.redis.Del(ctx, pfx.PrefixKeys(keys)...).Err(); err != nil {
		return nil, toGRPCError("plugin redis del", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) HGet(ctx context.Context, req *pluginsdk.RedisHGetRequest) (*pluginsdk.RedisValueResponse, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	val, err := s.redis.HGet(ctx, pfx.PrefixKey(req.GetKey()), req.GetField()).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return &pluginsdk.RedisValueResponse{Exists: false}, nil
		}
		return nil, toGRPCError("plugin redis hget", err)
	}
	return &pluginsdk.RedisValueResponse{Exists: true, Value: val}, nil
}

func (s *SDKServer) HSet(ctx context.Context, req *pluginsdk.RedisHSetRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.redis.HSet(ctx, pfx.PrefixKey(req.GetKey()), req.GetField(), req.GetValue()).Err(); err != nil {
		return nil, toGRPCError("plugin redis hset", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) HGetAll(ctx context.Context, req *pluginsdk.RedisKeyRequest) (*pluginsdk.RedisMapResponse, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.redis.HGetAll(ctx, pfx.PrefixKey(req.GetKey())).Result()
	if err != nil {
		return nil, toGRPCError("plugin redis hgetall", err)
	}
	out := &pluginsdk.RedisMapResponse{Fields: make(map[string][]byte, len(m))}
	for k, v := range m {
		out.Fields[k] = []byte(v)
	}
	return out, nil
}

func (s *SDKServer) HDel(ctx context.Context, req *pluginsdk.RedisHDelRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.redis.HDel(ctx, pfx.PrefixKey(req.GetKey()), req.GetFields()...).Err(); err != nil {
		return nil, toGRPCError("plugin redis hdel", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) Publish(ctx context.Context, req *pluginsdk.RedisPubRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pfx, err := s.redisPrefixer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.redis.Publish(ctx, pfx.PrefixChannel(req.GetChannel()), req.GetMessage()).Err(); err != nil {
		return nil, toGRPCError("plugin redis publish", err)
	}
	return &emptypb.Empty{}, nil
}

// Subscribe streams messages from Redis pub/sub channels. Channel names are
// automatically prefixed per plugin; returned messages restore original names.
func (s *SDKServer) Subscribe(req *pluginsdk.RedisSubRequest, stream grpc.ServerStreamingServer[pluginsdk.RedisMessage]) error {
	if s.redis == nil {
		return status.Error(codes.FailedPrecondition, "redis not configured")
	}
	channels := req.GetChannels()
	if len(channels) == 0 {
		return status.Error(codes.InvalidArgument, "at least one channel is required")
	}
	pfx, err := s.redisPrefixer(stream.Context())
	if err != nil {
		return err
	}
	return s.streamRedisSubscription(stream, pfx, channels)
}

// streamRedisSubscription handles the pub/sub loop for Subscribe.
func (s *SDKServer) streamRedisSubscription(
	stream grpc.ServerStreamingServer[pluginsdk.RedisMessage],
	pfx *RedisKeyPrefixer,
	channels []string,
) error {
	prefixed := pfx.PrefixChannels(channels)
	pubsub := s.redis.Subscribe(stream.Context(), prefixed...)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&pluginsdk.RedisMessage{
				Channel: pfx.StripChannelPrefix(msg.Channel),
				Data:    []byte(msg.Payload),
			}); err != nil {
				return err
			}
		}
	}
}

// ============================================================
// EventBus -- reuses Redis pub/sub as transport
// ============================================================

// eventBusChannelPrefix isolates eventbus channels from raw RedisProxy channels.
const eventBusChannelPrefix = "plugin_eventbus:"

// eventBusAdapter wraps SDKServer to avoid method signature conflict with RedisProxy.Publish.
type eventBusAdapter struct {
	pluginsdk.UnimplementedEventBusServer
	srv *SDKServer
}

// Publish sends an event on the calling plugin's eventbus namespace. Topics are
// scoped per plugin: a plugin can only publish to its own namespace, identified
// by the plugin name extracted from gRPC metadata.
func (a *eventBusAdapter) Publish(ctx context.Context, req *pluginsdk.EventRequest) (*emptypb.Empty, error) {
	if a.srv.redis == nil {
		return nil, status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pluginName, err := pluginNameFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "plugin identity: %v", err)
	}
	channel := eventBusChannelPrefix + pluginName + ":" + req.GetTopic()
	if err := a.srv.redis.Publish(ctx, channel, req.GetPayload()).Err(); err != nil {
		return nil, toGRPCError("plugin eventbus publish", err)
	}
	return &emptypb.Empty{}, nil
}

func (a *eventBusAdapter) Subscribe(req *pluginsdk.EventFilter, stream grpc.ServerStreamingServer[pluginsdk.Event]) error {
	return a.srv.eventBusSubscribe(req, stream)
}

// eventBusSubscribe streams events from the calling plugin's namespace. By
// default a plugin subscribes to its own topics; cross-plugin subscription is
// supported only by namespacing under the caller's name (see PluginNamespace
// override in EventFilter for future cross-plugin extensions).
func (s *SDKServer) eventBusSubscribe(req *pluginsdk.EventFilter, stream grpc.ServerStreamingServer[pluginsdk.Event]) error {
	if s.redis == nil {
		return status.Error(codes.FailedPrecondition, "redis not configured")
	}
	pluginName, err := pluginNameFromContext(stream.Context())
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "plugin identity: %v", err)
	}
	topics := req.GetTopics()
	if len(topics) == 0 {
		return status.Error(codes.InvalidArgument, "at least one topic is required")
	}
	channels, chMap := buildEventBusChannelMap(pluginName, topics)
	// TODO(proto): inject from_plugin into Event message once the proto adds the field.
	return s.streamEventBus(stream, channels, chMap)
}

// buildEventBusChannelMap maps eventbus topic names to prefixed Redis channel
// names scoped under the given plugin name.
func buildEventBusChannelMap(pluginName string, topics []string) ([]string, map[string]string) {
	channels := make([]string, 0, len(topics))
	chMap := make(map[string]string, len(topics))
	for _, t := range topics {
		ch := eventBusChannelPrefix + pluginName + ":" + t
		channels = append(channels, ch)
		chMap[ch] = t
	}
	return channels, chMap
}

// streamEventBus handles the pub/sub loop for EventBus.Subscribe.
func (s *SDKServer) streamEventBus(
	stream grpc.ServerStreamingServer[pluginsdk.Event],
	channels []string,
	chMap map[string]string,
) error {
	pubsub := s.redis.Subscribe(stream.Context(), channels...)
	defer pubsub.Close()

	msgCh := pubsub.Channel()
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case msg, ok := <-msgCh:
			if !ok {
				return nil
			}
			topic := chMap[msg.Channel]
			if topic == "" {
				topic = msg.Channel
			}
			if err := stream.Send(&pluginsdk.Event{
				Topic:     topic,
				Payload:   []byte(msg.Payload),
				Timestamp: time.Now().UnixMilli(),
			}); err != nil {
				return err
			}
		}
	}
}
