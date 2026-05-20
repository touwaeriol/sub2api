package plugin

import "fmt"

// redisPluginKeyPrefix is the format for plugin-scoped Redis key prefixes.
// All plugin Redis operations are automatically namespaced under this prefix.
const redisPluginKeyPrefix = "plugin:%s:"

// redisPluginChannelPrefix is the format for plugin-scoped pub/sub channel prefixes.
const redisPluginChannelPrefix = "plugin:%s:ch:"

// RedisKeyPrefixer adds per-plugin prefixes to Redis keys and channels,
// preventing plugins from accessing core or other plugins' data.
type RedisKeyPrefixer struct {
	keyPrefix     string
	channelPrefix string
}

// NewRedisKeyPrefixer creates a prefixer for the given plugin name.
func NewRedisKeyPrefixer(pluginName string) *RedisKeyPrefixer {
	return &RedisKeyPrefixer{
		keyPrefix:     fmt.Sprintf(redisPluginKeyPrefix, pluginName),
		channelPrefix: fmt.Sprintf(redisPluginChannelPrefix, pluginName),
	}
}

// PrefixKey adds the plugin namespace prefix to a key.
func (p *RedisKeyPrefixer) PrefixKey(key string) string {
	return p.keyPrefix + key
}

// PrefixKeys adds the plugin namespace prefix to each key.
func (p *RedisKeyPrefixer) PrefixKeys(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = p.keyPrefix + k
	}
	return out
}

// PrefixChannel adds the plugin namespace prefix to a pub/sub channel.
func (p *RedisKeyPrefixer) PrefixChannel(channel string) string {
	return p.channelPrefix + channel
}

// PrefixChannels adds the plugin namespace prefix to each channel.
func (p *RedisKeyPrefixer) PrefixChannels(channels []string) []string {
	out := make([]string, len(channels))
	for i, ch := range channels {
		out[i] = p.channelPrefix + ch
	}
	return out
}

// StripChannelPrefix removes the plugin namespace prefix from a channel name,
// so the plugin sees its original channel name in received messages.
func (p *RedisKeyPrefixer) StripChannelPrefix(channel string) string {
	if len(channel) > len(p.channelPrefix) {
		return channel[len(p.channelPrefix):]
	}
	return channel
}
