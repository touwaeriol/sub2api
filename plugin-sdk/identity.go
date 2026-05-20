package pluginsdk

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// pluginNameMDKey is the gRPC metadata key that carries the plugin's name.
// Must match the server-side constant in backend/internal/plugin/sdk_interceptor.go.
const pluginNameMDKey = "x-plugin-name"

// corePluginNameConfigKey is the config key the core injects to pass the plugin name.
// Must match the server-side constant.
const corePluginNameConfigKey = "__core_plugin_name__"

// pluginIdentityUnaryInterceptor returns a client interceptor that appends
// the plugin identity metadata to every unary RPC.
func pluginIdentityUnaryInterceptor(md metadata.MD) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// pluginIdentityStreamInterceptor returns a client interceptor that appends
// the plugin identity metadata to every streaming RPC.
func pluginIdentityStreamInterceptor(md metadata.MD) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = metadata.NewOutgoingContext(ctx, md)
		return streamer(ctx, desc, cc, method, opts...)
	}
}
