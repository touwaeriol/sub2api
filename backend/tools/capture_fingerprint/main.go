// capture_fingerprint spins up a local HTTPS+H2 server that records the
// TLS ClientHello and the first HTTP/2 frames sent by the connecting client,
// then replies with a minimal valid Anthropic /v1/messages SSE stream so the
// real Claude Code CLI treats the exchange as successful.
//
// Usage:
//
//	go run ./tools/capture_fingerprint -addr 127.0.0.1:8443 -out fp.json
//
// Then, from another shell, point Claude Code at it:
//
//	export ANTHROPIC_BASE_URL="https://localhost:8443"
//	export ANTHROPIC_API_KEY="sk-ant-capture-dummy"
//	export NODE_TLS_REJECT_UNAUTHORIZED=0
//	echo 'hi' | claude -p --model claude-sonnet-4-5 >/dev/null
//
// After the CLI call returns, the server prints the captured fingerprint as
// JSON to stdout and (optionally) writes it to the -out file.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	ctls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8443", "listen address")
	outFile := flag.String("out", "", "optional: write each capture JSON to this file (overwrites on every connection)")
	outDir := flag.String("out-dir", "", "optional: write each capture as capture_<rfc3339>_<n>.json into this dir (does not overwrite)")
	flag.Parse()

	var captureCounter int64
	var counterMu sync.Mutex
	nextCaptureIdx := func() int64 {
		counterMu.Lock()
		defer counterMu.Unlock()
		captureCounter++
		return captureCounter
	}

	cert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("generate cert: %v", err)
	}

	tlsCfg := &ctls.Config{
		Certificates: []ctls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
		MinVersion:   ctls.VersionTLS12,
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	defer ln.Close()

	log.Printf("capture server listening on https://%s", *addr)
	log.Printf("run in another shell:")
	log.Printf("  NODE_TLS_REJECT_UNAUTHORIZED=0 \\")
	log.Printf("  ANTHROPIC_BASE_URL=https://localhost:%s \\", portOf(*addr))
	log.Printf("  ANTHROPIC_API_KEY=sk-ant-capture-dummy \\")
	log.Printf("  claude -p --model claude-sonnet-4-5 'hi'")
	log.Printf("")

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("accept: %v", err)
			continue
		}

		go func(raw net.Conn) {
			defer raw.Close()
			_ = raw.SetDeadline(time.Now().Add(30 * time.Second))

			capture := &Capture{
				CapturedAt: time.Now().UTC().Format(time.RFC3339),
				RemoteAddr: raw.RemoteAddr().String(),
			}
			idx := nextCaptureIdx()
			// Always dump whatever we captured, even if handshake / h2 fails later.
			defer func() { printCapture(capture, *outFile, *outDir, idx) }()

			// Step 1: sniff the first ClientHello record so we can parse
			// extensions / cipher order before the TLS library eats them.
			peeker := newPeekConn(raw)
			helloBytes, err := readTLSHandshakeRecord(peeker)
			if err != nil {
				log.Printf("read ClientHello: %v", err)
				return
			}
			capture.ClientHelloRaw = hex.EncodeToString(helloBytes)
			if err := parseClientHello(helloBytes, capture); err != nil {
				log.Printf("parse ClientHello: %v", err)
			}

			// Step 2: do the actual TLS handshake using the sniffed bytes
			// replayed back to crypto/tls via our peekConn buffer.
			tlsConn := ctls.Server(peeker, tlsCfg)
			hsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tlsConn.HandshakeContext(hsCtx); err != nil {
				log.Printf("tls handshake: %v", err)
				return
			}
			state := tlsConn.ConnectionState()
			capture.NegotiatedProto = state.NegotiatedProtocol
			capture.TLSVersion = tlsVersionName(state.Version)
			capture.ServerName = state.ServerName

			switch state.NegotiatedProtocol {
			case "h2":
				if err := serveH2(tlsConn, capture); err != nil {
					log.Printf("h2: %v", err)
				}
			case "http/1.1", "":
				if err := serveH1(tlsConn, capture); err != nil {
					log.Printf("h1: %v", err)
				}
			default:
				log.Printf("unknown ALPN: %q", state.NegotiatedProtocol)
			}
		}(conn)
	}
}

// ================== Capture struct ==================

type Capture struct {
	CapturedAt       string              `json:"captured_at"`
	RemoteAddr       string              `json:"remote_addr"`
	ServerName       string              `json:"server_name,omitempty"`
	TLSVersion       string              `json:"tls_version"`
	NegotiatedProto  string              `json:"negotiated_proto"`
	ClientHelloRaw   string              `json:"client_hello_raw"`
	CipherSuites     []string            `json:"cipher_suites"`
	Extensions       []string            `json:"extensions"`
	Curves           []string            `json:"curves"`
	PointFormats     []string            `json:"point_formats"`
	SignatureAlgos   []string            `json:"signature_algorithms"`
	SupportedVersions []string           `json:"supported_versions"`
	KeyShareGroups   []string            `json:"key_share_groups"`
	ALPNProtos       []string            `json:"alpn_protos"`
	PSKModes         []string            `json:"psk_modes"`
	JA3String        string              `json:"ja3_string"`
	JA3Hash          string              `json:"ja3_hash"`
	JA4              string              `json:"ja4"`

	HTTP2 *H2Capture `json:"http2,omitempty"`
	HTTP1 *H1Capture `json:"http1,omitempty"`
}

type H2Capture struct {
	PrefaceSeen             bool              `json:"preface_seen"`
	ClientSettings          map[string]uint32 `json:"client_settings"`
	WindowUpdate            *uint32           `json:"initial_window_update,omitempty"`
	FirstHeadersPseudoOrder []string          `json:"first_headers_pseudo_order"`
	FirstHeadersAll         []string          `json:"first_headers_all"`
	FrameSequence           []string          `json:"frame_sequence"`
	// Requests records every HTTP/2 request (stream) the client opened on
	// this connection, in order seen. Used to discover what sidecar
	// endpoints real Claude Code calls beyond /v1/messages.
	Requests []H2Request `json:"requests,omitempty"`
}

// H2Request is one HTTP/2 request seen on a capture session.
type H2Request struct {
	StreamID    uint32   `json:"stream_id"`
	Method      string   `json:"method"`
	Authority   string   `json:"authority,omitempty"`
	Path        string   `json:"path"`
	Headers     []string `json:"headers,omitempty"`
	BodyPreview string   `json:"body_preview,omitempty"`
}

type H1Capture struct {
	RequestLine string   `json:"request_line"`
	Headers     []string `json:"headers_in_order"`
	BodyPreview string   `json:"body_preview,omitempty"`
	BodyBytes   int      `json:"body_bytes,omitempty"`
}

// ================== peekConn (replay ClientHello) ==================

// peekConn buffers bytes read from the underlying conn so they can be
// replayed to crypto/tls after we've inspected them for fingerprinting.
type peekConn struct {
	net.Conn
	buf     *bytes.Buffer
	drained bool
	mu      sync.Mutex
}

func newPeekConn(c net.Conn) *peekConn {
	return &peekConn{Conn: c, buf: &bytes.Buffer{}}
}

// peek reads n bytes from the underlying conn and records them.
func (p *peekConn) peek(n int) ([]byte, error) {
	chunk := make([]byte, n)
	if _, err := io.ReadFull(p.Conn, chunk); err != nil {
		return nil, err
	}
	p.buf.Write(chunk)
	return chunk, nil
}

func (p *peekConn) Read(b []byte) (int, error) {
	p.mu.Lock()
	if !p.drained && p.buf.Len() > 0 {
		n, _ := p.buf.Read(b)
		if p.buf.Len() == 0 {
			p.drained = true
		}
		p.mu.Unlock()
		return n, nil
	}
	p.mu.Unlock()
	return p.Conn.Read(b)
}

// readTLSHandshakeRecord reads a single TLS record (5-byte header + fragment)
// and returns the raw handshake message bytes (the handshake payload, i.e.
// ClientHello starting with the handshake-type byte 0x01).
func readTLSHandshakeRecord(p *peekConn) ([]byte, error) {
	header, err := p.peek(5)
	if err != nil {
		return nil, err
	}
	if header[0] != 0x16 { // not a handshake record
		return nil, fmt.Errorf("not a tls handshake record: first byte=0x%02x", header[0])
	}
	length := int(header[3])<<8 | int(header[4])
	if length <= 0 || length > 16*1024 {
		return nil, fmt.Errorf("bad handshake record length %d", length)
	}
	fragment, err := p.peek(length)
	if err != nil {
		return nil, err
	}
	return fragment, nil
}

// ================== ClientHello parsing via utls Fingerprinter ==================

// wrapAsRecord wraps a raw handshake fragment back into a TLS record so
// utls.Fingerprinter can consume it. Fingerprinter expects the 5-byte TLS
// record header to be present.
func wrapAsRecord(fragment []byte) []byte {
	length := len(fragment)
	return append([]byte{0x16, 0x03, 0x01, byte(length >> 8), byte(length)}, fragment...)
}

func parseClientHello(fragment []byte, c *Capture) error {
	fp := &utls.Fingerprinter{AllowBluntMimicry: true}
	spec, err := fp.FingerprintClientHello(wrapAsRecord(fragment))
	if err != nil {
		return fmt.Errorf("utls fingerprint: %w", err)
	}

	for _, cs := range spec.CipherSuites {
		c.CipherSuites = append(c.CipherSuites, fmt.Sprintf("0x%04x", cs))
	}
	for _, ext := range spec.Extensions {
		c.Extensions = append(c.Extensions, extensionName(ext))
		collectExtensionDetails(ext, c)
	}

	// JA3 = TLSVersion,Ciphers,Extensions,Curves,PointFormats  (decimal, dashes, commas)
	ja3 := fmt.Sprintf("%d,%s,%s,%s,%s",
		771, // TLS 1.2 legacy version byte — JA3 convention
		joinDecUint16(spec.CipherSuites),
		joinDecExtensions(spec.Extensions),
		joinDecCurves(c.Curves),
		joinDecPointFormats(c.PointFormats),
	)
	c.JA3String = ja3
	c.JA3Hash = md5hex(ja3)

	c.JA4 = computeJA4(spec, c)
	return nil
}

func collectExtensionDetails(ext utls.TLSExtension, c *Capture) {
	switch x := ext.(type) {
	case *utls.SupportedCurvesExtension:
		for _, cv := range x.Curves {
			c.Curves = append(c.Curves, fmt.Sprintf("0x%04x", uint16(cv)))
		}
	case *utls.SupportedPointsExtension:
		for _, pf := range x.SupportedPoints {
			c.PointFormats = append(c.PointFormats, fmt.Sprintf("0x%02x", pf))
		}
	case *utls.SignatureAlgorithmsExtension:
		for _, sa := range x.SupportedSignatureAlgorithms {
			c.SignatureAlgos = append(c.SignatureAlgos, fmt.Sprintf("0x%04x", uint16(sa)))
		}
	case *utls.SupportedVersionsExtension:
		for _, v := range x.Versions {
			c.SupportedVersions = append(c.SupportedVersions, fmt.Sprintf("0x%04x", v))
		}
	case *utls.KeyShareExtension:
		for _, ks := range x.KeyShares {
			c.KeyShareGroups = append(c.KeyShareGroups, fmt.Sprintf("0x%04x", uint16(ks.Group)))
		}
	case *utls.ALPNExtension:
		c.ALPNProtos = append(c.ALPNProtos, x.AlpnProtocols...)
	case *utls.PSKKeyExchangeModesExtension:
		for _, m := range x.Modes {
			c.PSKModes = append(c.PSKModes, fmt.Sprintf("0x%02x", m))
		}
	}
}

func extensionName(ext utls.TLSExtension) string {
	// Best-effort human label via type name.
	switch x := ext.(type) {
	case *utls.SNIExtension:
		_ = x
		return "server_name (0)"
	case *utls.StatusRequestExtension:
		return "status_request (5)"
	case *utls.SupportedCurvesExtension:
		return "supported_groups (10)"
	case *utls.SupportedPointsExtension:
		return "ec_point_formats (11)"
	case *utls.SignatureAlgorithmsExtension:
		return "signature_algorithms (13)"
	case *utls.ALPNExtension:
		return "application_layer_protocol_negotiation (16)"
	case *utls.SCTExtension:
		return "signed_certificate_timestamp (18)"
	case *utls.ExtendedMasterSecretExtension:
		return "extended_master_secret (23)"
	case *utls.UtlsCompressCertExtension:
		return "compress_certificate (27)"
	case *utls.SessionTicketExtension:
		return "session_ticket (35)"
	case *utls.SupportedVersionsExtension:
		return "supported_versions (43)"
	case *utls.PSKKeyExchangeModesExtension:
		return "psk_key_exchange_modes (45)"
	case *utls.KeyShareExtension:
		return "key_share (51)"
	case *utls.UtlsGREASEExtension:
		return "GREASE (0x0a0a / 0xdada etc.)"
	case *utls.UtlsPaddingExtension:
		return "padding (21)"
	case *utls.RenegotiationInfoExtension:
		return "renegotiation_info (0xff01)"
	case *utls.GenericExtension:
		return fmt.Sprintf("generic (0x%04x)", x.Id)
	}
	return fmt.Sprintf("%T", ext)
}

// ================== HTTP/2 handling ==================

const h2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// serveH2 handles an HTTP/2 session in "discovery" mode: it captures the
// TLS/H2 fingerprint on the first request and then keeps the connection open
// responding to every subsequent request with a path-appropriate stub, so we
// can observe the full set of endpoints a real Claude Code CLI hits in one
// session (not just /v1/messages).
//
// Exit conditions: client GOAWAY, read deadline (idle), or explicit error.
// The overall session is bounded by a 60s deadline ceiling on the raw conn
// applied by the caller; per-read we use a 15s idle timeout.
func serveH2(tlsConn *ctls.Conn, c *Capture) error {
	c.HTTP2 = &H2Capture{ClientSettings: map[string]uint32{}}
	reader := bufio.NewReader(tlsConn)

	// Read preface
	preface := make([]byte, len(h2Preface))
	if _, err := io.ReadFull(reader, preface); err != nil {
		return fmt.Errorf("read preface: %w", err)
	}
	if string(preface) != h2Preface {
		return fmt.Errorf("bad preface: %q", preface)
	}
	c.HTTP2.PrefaceSeen = true

	framer := http2.NewFramer(tlsConn, reader)
	framer.SetMaxReadFrameSize(16384)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)

	// Server's SETTINGS frame (initial)
	if err := framer.WriteSettings(); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	firstHeadersSeen := false
	// Track DATA body fragments per stream so we can surface the first N
	// bytes of each request body in the capture output. Useful for
	// inspecting what count_tokens / messages payloads look like.
	bodies := map[uint32]*bytes.Buffer{}
	const bodyPreviewLimit = 512

	for {
		// Per-frame idle timeout. 30s is long enough for Claude Code to
		// finish displaying a streamed response before sending its next
		// request, but short enough that a hung session dumps its capture
		// in reasonable wall-clock time.
		_ = tlsConn.SetDeadline(time.Now().Add(30 * time.Second))
		frame, err := framer.ReadFrame()
		if err != nil {
			// Idle timeout or connection close is the expected happy path
			// after the CLI is done. Report as nil error so the capture
			// dump still happens.
			return nil
		}
		c.HTTP2.FrameSequence = append(c.HTTP2.FrameSequence, frame.Header().Type.String())

		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				_ = f.ForeachSetting(func(s http2.Setting) error {
					c.HTTP2.ClientSettings[settingName(s.ID)] = s.Val
					return nil
				})
				if err := framer.WriteSettingsAck(); err != nil {
					return fmt.Errorf("write settings ack: %w", err)
				}
			}
		case *http2.WindowUpdateFrame:
			inc := f.Increment
			if c.HTTP2.WindowUpdate == nil {
				c.HTTP2.WindowUpdate = &inc
			}
		case *http2.MetaHeadersFrame:
			req := H2Request{StreamID: f.StreamID}
			var pseudo, all []string
			for _, hf := range f.Fields {
				all = append(all, hf.Name+": "+hf.Value)
				if hf.IsPseudo() {
					pseudo = append(pseudo, hf.Name)
				}
				switch hf.Name {
				case ":method":
					req.Method = hf.Value
				case ":path":
					req.Path = hf.Value
				case ":authority":
					req.Authority = hf.Value
				}
			}
			req.Headers = all
			// Record the first request's header ordering for fingerprint
			// comparison (backwards compatible with the old capture format).
			if !firstHeadersSeen {
				c.HTTP2.FirstHeadersPseudoOrder = pseudo
				c.HTTP2.FirstHeadersAll = all
				firstHeadersSeen = true
			}
			c.HTTP2.Requests = append(c.HTTP2.Requests, req)
			// Emit the stub response immediately. Claude Code won't send
			// the body until it sees the request headers are accepted, but
			// for requests with bodies (POST /v1/messages etc) the DataFrame
			// will arrive after we've already written our response header
			// block — that's fine, we just absorb the DATA frames below.
			if err := writeStubResponseForPath(framer, f.StreamID, req.Path); err != nil {
				// Don't bail on write errors: the client may have already
				// closed the stream. Just log and continue.
				log.Printf("write stub for %s: %v", req.Path, err)
			}
		case *http2.DataFrame:
			buf, ok := bodies[f.StreamID]
			if !ok {
				buf = &bytes.Buffer{}
				bodies[f.StreamID] = buf
			}
			if buf.Len() < bodyPreviewLimit {
				need := bodyPreviewLimit - buf.Len()
				data := f.Data()
				if len(data) > need {
					data = data[:need]
				}
				buf.Write(data)
			}
			if f.StreamEnded() {
				// Attach body preview back to the matching request.
				for i := range c.HTTP2.Requests {
					if c.HTTP2.Requests[i].StreamID == f.StreamID {
						c.HTTP2.Requests[i].BodyPreview = buf.String()
						break
					}
				}
			}
		case *http2.PingFrame:
			// Reply to client pings so idle-detection keepalives work.
			if !f.IsAck() {
				_ = framer.WritePing(true, f.Data)
			}
		case *http2.GoAwayFrame:
			return nil
		}
	}
}

// writeStubResponseForPath dispatches to a path-appropriate stub. Unknown
// paths get a generic 404 so the CLI visibly errors on them — which is fine
// for a diagnostic tool; the path is already recorded in the capture.
func writeStubResponseForPath(framer *http2.Framer, streamID uint32, path string) error {
	switch {
	case path == "" || path == "/":
		return writeFakeJSONResponse(framer, streamID, 404, `{"error":"empty path"}`)
	case strings.HasSuffix(path, "/count_tokens") || strings.Contains(path, "/count_tokens?"):
		return writeFakeJSONResponse(framer, streamID, 200, `{"input_tokens":1}`)
	case strings.HasPrefix(path, "/v1/messages"):
		return writeFakeH2Response(framer, streamID)
	case strings.HasPrefix(path, "/api/oauth/usage") || strings.HasPrefix(path, "/v1/organizations"):
		return writeFakeJSONResponse(framer, streamID, 200, `{"five_hour":{"utilization":0.05,"resets_at":"2099-01-01T00:00:00Z"},"seven_day":{"utilization":0.05,"resets_at":"2099-01-01T00:00:00Z"},"seven_day_sonnet":{"utilization":0.05,"resets_at":"2099-01-01T00:00:00Z"}}`)
	case strings.HasPrefix(path, "/v1/models"):
		return writeFakeJSONResponse(framer, streamID, 200, `{"data":[],"has_more":false,"first_id":null,"last_id":null}`)
	default:
		return writeFakeJSONResponse(framer, streamID, 404, `{"type":"error","error":{"type":"not_found_error","message":"capture stub has no handler for this path"}}`)
	}
}

// writeFakeJSONResponse sends a single-frame HTTP/2 JSON response with the
// given status and body. Used for non-streaming endpoints like
// count_tokens, /api/oauth/usage, /v1/models.
func writeFakeJSONResponse(framer *http2.Framer, streamID uint32, status int, body string) error {
	if streamID == 0 {
		streamID = 1
	}
	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: fmt.Sprintf("%d", status)})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/json"})
	_ = enc.WriteField(hpack.HeaderField{Name: "x-request-id", Value: "req_capture_stub"})

	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hdrBuf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	}); err != nil {
		return err
	}
	return framer.WriteData(streamID, true, []byte(body))
}

func writeFakeH2Response(framer *http2.Framer, streamID uint32) error {
	if streamID == 0 {
		streamID = 1
	}
	// Encode a minimal response header block via hpack.
	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "text/event-stream"})
	_ = enc.WriteField(hpack.HeaderField{Name: "cache-control", Value: "no-cache"})
	_ = enc.WriteField(hpack.HeaderField{Name: "x-request-id", Value: "req_capture_001"})

	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hdrBuf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	}); err != nil {
		return err
	}

	// Minimal SSE body that Claude Code will parse as a valid stream.
	body := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_capture","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	if err := framer.WriteData(streamID, true, []byte(body)); err != nil {
		return err
	}
	return nil
}

func settingName(id http2.SettingID) string {
	switch id {
	case http2.SettingHeaderTableSize:
		return "HEADER_TABLE_SIZE"
	case http2.SettingEnablePush:
		return "ENABLE_PUSH"
	case http2.SettingMaxConcurrentStreams:
		return "MAX_CONCURRENT_STREAMS"
	case http2.SettingInitialWindowSize:
		return "INITIAL_WINDOW_SIZE"
	case http2.SettingMaxFrameSize:
		return "MAX_FRAME_SIZE"
	case http2.SettingMaxHeaderListSize:
		return "MAX_HEADER_LIST_SIZE"
	}
	return fmt.Sprintf("SETTING_0x%04x", uint16(id))
}

// ================== HTTP/1.1 fallback ==================

func serveH1(tlsConn *ctls.Conn, c *Capture) error {
	c.HTTP1 = &H1Capture{}
	r := bufio.NewReader(tlsConn)

	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	c.HTTP1.RequestLine = trimCRLF(line)

	contentLength := 0
	for {
		h, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		h = trimCRLF(h)
		if h == "" {
			break
		}
		c.HTTP1.Headers = append(c.HTTP1.Headers, h)
		if strings.HasPrefix(strings.ToLower(h), "content-length:") {
			v := strings.TrimSpace(h[len("content-length:"):])
			_, _ = fmt.Sscanf(v, "%d", &contentLength)
		}
	}

	// Capture request body (up to 256KB preview). Drain the rest so the write
	// below doesn't race the client's send.
	const bodyPreviewMax = 256 * 1024
	if contentLength > 0 {
		c.HTTP1.BodyBytes = contentLength
		previewLen := contentLength
		if previewLen > bodyPreviewMax {
			previewLen = bodyPreviewMax
		}
		preview := make([]byte, previewLen)
		if _, err := io.ReadFull(r, preview); err == nil {
			c.HTTP1.BodyPreview = string(preview)
			// Drain the remaining body (don't care about the data)
			remaining := contentLength - previewLen
			if remaining > 0 {
				_, _ = io.CopyN(io.Discard, r, int64(remaining))
			}
		}
	}

	// Minimal valid Anthropic SSE — provide both input_tokens and output_tokens
	// in message_delta usage so CC's SDK doesn't choke on undefined.input_tokens.
	body := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_capture","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":1,"output_tokens":1}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	resp := "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nCache-Control: no-cache\r\nConnection: close\r\n\r\n" + body
	_, _ = tlsConn.Write([]byte(resp))
	return nil
}

// ================== helpers ==================

func printCapture(c *Capture, outFile, outDir string, idx int64) {
	data, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println(string(data))
	if outFile != "" {
		if err := os.WriteFile(outFile, data, 0o644); err != nil {
			log.Printf("write %s: %v", outFile, err)
		} else {
			log.Printf("wrote fingerprint to %s", outFile)
		}
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			log.Printf("mkdir %s: %v", outDir, err)
			return
		}
		// Filename: capture_<unix-epoch-ms>_<idx>.json — sortable and unique.
		fname := fmt.Sprintf("capture_%d_%03d.json", time.Now().UnixMilli(), idx)
		full := outDir + "/" + fname
		if err := os.WriteFile(full, data, 0o644); err != nil {
			log.Printf("write %s: %v", full, err)
		} else {
			log.Printf("wrote capture #%d to %s", idx, full)
		}
	}
}

func trimCRLF(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func portOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}

func tlsVersionName(v uint16) string {
	switch v {
	case ctls.VersionTLS13:
		return "TLS 1.3"
	case ctls.VersionTLS12:
		return "TLS 1.2"
	}
	return fmt.Sprintf("0x%04x", v)
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// ================== JA3 / JA4 formatting ==================

// IsGREASE returns true if the value is one of the GREASE reserved values.
func isGrease(v uint16) bool {
	return (v & 0x0f0f) == 0x0a0a && (v>>8) == (v&0xff)
}

func joinDecUint16(vals []uint16) string {
	out := ""
	first := true
	for _, v := range vals {
		if isGrease(v) {
			continue
		}
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", v)
		first = false
	}
	return out
}

// joinDecExtensions extracts extension IDs (in order, GREASE-stripped) from a
// slice of utls.TLSExtension and joins them into JA3 format.
func joinDecExtensions(exts []utls.TLSExtension) string {
	out := ""
	first := true
	for _, ext := range exts {
		id, ok := extensionID(ext)
		if !ok {
			continue
		}
		if isGrease(id) {
			continue
		}
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", id)
		first = false
	}
	return out
}

func extensionID(ext utls.TLSExtension) (uint16, bool) {
	switch x := ext.(type) {
	case *utls.SNIExtension:
		return 0, true
	case *utls.StatusRequestExtension:
		return 5, true
	case *utls.SupportedCurvesExtension:
		return 10, true
	case *utls.SupportedPointsExtension:
		return 11, true
	case *utls.SignatureAlgorithmsExtension:
		return 13, true
	case *utls.ALPNExtension:
		return 16, true
	case *utls.SCTExtension:
		return 18, true
	case *utls.ExtendedMasterSecretExtension:
		return 23, true
	case *utls.UtlsCompressCertExtension:
		return 27, true
	case *utls.SessionTicketExtension:
		return 35, true
	case *utls.SupportedVersionsExtension:
		return 43, true
	case *utls.PSKKeyExchangeModesExtension:
		return 45, true
	case *utls.KeyShareExtension:
		return 51, true
	case *utls.RenegotiationInfoExtension:
		return 0xff01, true
	case *utls.UtlsPaddingExtension:
		return 21, true
	case *utls.GenericExtension:
		return x.Id, true
	}
	return 0, false
}

func joinDecCurves(hexes []string) string {
	out := ""
	first := true
	for _, h := range hexes {
		var v uint16
		_, _ = fmt.Sscanf(h, "0x%04x", &v)
		if isGrease(v) {
			continue
		}
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", v)
		first = false
	}
	return out
}

func joinDecPointFormats(hexes []string) string {
	out := ""
	first := true
	for _, h := range hexes {
		var v uint8
		_, _ = fmt.Sscanf(h, "0x%02x", &v)
		if !first {
			out += "-"
		}
		out += fmt.Sprintf("%d", v)
		first = false
	}
	return out
}

// computeJA4 is a best-effort JA4 TLS fingerprint following
// https://github.com/FoxIO-LLC/ja4 spec. We do NOT compute the full raw/hash
// suffixes (they need extra sorting); we fill in the part-a ("t13d1714h2")
// prefix accurately and leave part-b/c for manual use against a reference.
func computeJA4(spec *utls.ClientHelloSpec, c *Capture) string {
	// Part a: tNNdCCxxHH
	version := "13"
	if spec == nil {
		return ""
	}
	// Transport is 't' for TCP.
	// Count cipher suites (excluding GREASE).
	ccCount := 0
	for _, v := range spec.CipherSuites {
		if !isGrease(v) {
			ccCount++
		}
	}
	// Count extensions (excluding GREASE, but including SNI/ALPN).
	xxCount := 0
	hasALPN := false
	firstALPN := "00"
	for _, ext := range spec.Extensions {
		id, ok := extensionID(ext)
		if !ok || isGrease(id) {
			continue
		}
		xxCount++
		if id == 16 {
			hasALPN = true
			if ax, ok := ext.(*utls.ALPNExtension); ok && len(ax.AlpnProtocols) > 0 {
				first := ax.AlpnProtocols[0]
				if len(first) >= 2 {
					firstALPN = first[:1] + first[len(first)-1:]
				}
			}
		}
	}
	_ = hasALPN
	return fmt.Sprintf("t%sd%02d%02d%s", version, ccCount, xxCount, firstALPN)
}

// ================== Self-signed cert ==================

func generateSelfSignedCert() (ctls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return ctls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return ctls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "api.anthropic.com",
			Organization: []string{"sub2api capture"},
		},
		DNSNames:    []string{"api.anthropic.com", "localhost", "claude.ai"},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return ctls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return ctls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return ctls.X509KeyPair(certPEM, keyPEM)
}

