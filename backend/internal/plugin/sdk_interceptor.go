package plugin

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// pluginNameMDKey is the gRPC metadata key that carries the calling plugin's name.
// Plugins inject this via a client-side interceptor; the SDK server extracts it.
const pluginNameMDKey = "x-plugin-name"

// corePluginNameConfigKey is the config key the core injects into PluginInitRequest.Config
// so the plugin SDK can identify itself in subsequent gRPC calls.
const corePluginNameConfigKey = "__core_plugin_name__"

// pluginNameCtxKey is the context key for the extracted plugin name.
type pluginNameCtxKey struct{}

// pluginNameFromContext extracts the plugin name previously set by the SDK interceptor.
func pluginNameFromContext(ctx context.Context) (string, error) {
	name, ok := ctx.Value(pluginNameCtxKey{}).(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing plugin identity in gRPC metadata")
	}
	return name, nil
}

// sdkUnaryInterceptor extracts the plugin name from gRPC metadata and injects
// it into the request context. Returns an error if the metadata is missing.
func sdkUnaryInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	name, err := extractPluginName(ctx)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, pluginNameCtxKey{}, name)
	return handler(ctx, req)
}

// sdkStreamInterceptor wraps a server stream to inject the plugin name
// into the context for streaming RPCs (e.g. Subscribe).
func sdkStreamInterceptor(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	name, err := extractPluginName(ss.Context())
	if err != nil {
		return err
	}
	ctx := context.WithValue(ss.Context(), pluginNameCtxKey{}, name)
	return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
}

// extractPluginName reads the plugin name from incoming gRPC metadata.
func extractPluginName(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("no gRPC metadata; plugin identity required")
	}
	values := md.Get(pluginNameMDKey)
	if len(values) == 0 || values[0] == "" {
		return "", fmt.Errorf("missing %s in gRPC metadata", pluginNameMDKey)
	}
	return values[0], nil
}

// wrappedStream overrides Context() to return the enriched context.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
