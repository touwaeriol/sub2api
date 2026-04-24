package api

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// cryptoAPI adapts service.SecretEncryptor (AES-256-GCM, base64 text I/O)
// to the plugin.Crypto contract which speaks raw []byte. The service
// already performs random-nonce authenticated encryption, so the adapter
// only bridges the type mismatch.
type cryptoAPI struct {
	guard     *guard
	encryptor service.SecretEncryptor
}

// newCrypto returns the wrapper or an ErrNotImplemented stub.
func newCrypto(c *coreAPIImpl) plugin.Crypto {
	if c.deps.Encryptor == nil {
		return unimplementedCrypto{}
	}
	return &cryptoAPI{guard: c.guard, encryptor: c.deps.Encryptor}
}

// Encrypt encrypts plaintext with the host's shared symmetric key. The
// returned slice is the base64 ciphertext (the underlying service API
// emits text to ease DB storage); callers MUST round-trip it through
// Decrypt rather than reinterpreting bytes.
func (c *cryptoAPI) Encrypt(plaintext []byte) ([]byte, error) {
	if err := c.guard.requirePerm(plugin.PermCrypto); err != nil {
		return nil, err
	}
	enc, err := c.encryptor.Encrypt(string(plaintext))
	if err != nil {
		return nil, fmt.Errorf("plugin crypto encrypt: %w", err)
	}
	return []byte(enc), nil
}

// Decrypt reverses Encrypt.
func (c *cryptoAPI) Decrypt(ciphertext []byte) ([]byte, error) {
	if err := c.guard.requirePerm(plugin.PermCrypto); err != nil {
		return nil, err
	}
	pt, err := c.encryptor.Decrypt(string(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("plugin crypto decrypt: %w", err)
	}
	return []byte(pt), nil
}

// unimplementedCrypto fails every call when no encryptor is wired.
type unimplementedCrypto struct{}

func (unimplementedCrypto) Encrypt([]byte) ([]byte, error) { return nil, plugin.ErrNotImplemented }
func (unimplementedCrypto) Decrypt([]byte) ([]byte, error) { return nil, plugin.ErrNotImplemented }
