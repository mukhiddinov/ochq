// Package proto defines the OCHQ wire protocol used on the control
// connection between the ochq client and the ochqd server.
//
// The handshake runs on the raw TCP connection before it is handed over
// to yamux. Every message is a frame:
//
//	+----------------+---------------------------+
//	| length (4B BE) | JSON envelope (length B)  |
//	+----------------+---------------------------+
//
// Frames are read with io.ReadFull and no buffering, so no bytes belonging
// to the subsequent yamux session are consumed by the handshake reader.
package proto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Version is the current protocol version. The server rejects clients
// whose Hello.Version does not match.
const Version = 1

// MaxFrameSize bounds a single handshake frame to protect the server
// from unauthenticated clients announcing huge lengths.
const MaxFrameSize = 64 << 10

// HandshakeTimeout is the recommended deadline for completing the
// handshake on both sides.
const HandshakeTimeout = 10 * time.Second

// MessageType identifies the payload carried in an envelope.
type MessageType string

const (
	TypeHello         MessageType = "hello"
	TypeHelloResponse MessageType = "hello_response"
)

// TunnelType is the kind of traffic a tunnel carries.
type TunnelType string

const (
	TunnelHTTP TunnelType = "http"
)

// ErrorCode is a machine-readable reason for a rejected handshake.
// Clients use it to decide whether reconnecting makes sense.
type ErrorCode string

const (
	ErrCodeBadRequest       ErrorCode = "bad_request"
	ErrCodeVersionMismatch  ErrorCode = "version_mismatch"
	ErrCodeUnauthorized     ErrorCode = "unauthorized"
	ErrCodeInvalidSubdomain ErrorCode = "invalid_subdomain"
	ErrCodeSubdomainTaken   ErrorCode = "subdomain_taken"
	ErrCodeInternal         ErrorCode = "internal"
)

var (
	ErrFrameTooLarge  = errors.New("proto: frame exceeds maximum size")
	ErrUnexpectedType = errors.New("proto: unexpected message type")
)

// Hello is sent by the client right after dialing the server.
type Hello struct {
	Version       int        `json:"version"`
	Type          TunnelType `json:"type"`
	Subdomain     string     `json:"subdomain,omitempty"` // empty: server assigns a random one
	Token         string     `json:"token,omitempty"`
	ClientVersion string     `json:"client_version,omitempty"`
}

// HelloResponse is the server's answer to Hello. On success Error is
// empty and the connection switches to yamux immediately afterwards.
type HelloResponse struct {
	Subdomain string    `json:"subdomain,omitempty"`
	PublicURL string    `json:"public_url,omitempty"`
	Error     string    `json:"error,omitempty"`
	Code      ErrorCode `json:"code,omitempty"`
}

// OK reports whether the server accepted the handshake.
func (r *HelloResponse) OK() bool {
	return r.Error == "" && r.Code == ""
}

// Err converts a rejected response into an error, or returns nil.
func (r *HelloResponse) Err() error {
	if r.OK() {
		return nil
	}
	return &HandshakeError{Code: r.Code, Message: r.Error}
}

// HandshakeError is returned to the client when the server rejects Hello.
type HandshakeError struct {
	Code    ErrorCode
	Message string
}

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("handshake rejected (%s): %s", e.Code, e.Message)
}

type envelope struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// WriteHello writes a Hello frame to w.
func WriteHello(w io.Writer, h *Hello) error {
	return writeFrame(w, TypeHello, h)
}

// ReadHello reads a Hello frame from r.
func ReadHello(r io.Reader) (*Hello, error) {
	var h Hello
	if err := readFrame(r, TypeHello, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// WriteHelloResponse writes a HelloResponse frame to w.
func WriteHelloResponse(w io.Writer, resp *HelloResponse) error {
	return writeFrame(w, TypeHelloResponse, resp)
}

// ReadHelloResponse reads a HelloResponse frame from r.
func ReadHelloResponse(r io.Reader) (*HelloResponse, error) {
	var resp HelloResponse
	if err := readFrame(r, TypeHelloResponse, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func writeFrame(w io.Writer, typ MessageType, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("proto: marshal %s: %w", typ, err)
	}
	body, err := json.Marshal(envelope{Type: typ, Payload: payload})
	if err != nil {
		return fmt.Errorf("proto: marshal envelope: %w", err)
	}
	if len(body) > MaxFrameSize {
		return ErrFrameTooLarge
	}

	// Header and body go out in a single Write so a frame is never split
	// across writes by concurrent callers.
	buf := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(buf, uint32(len(body)))
	copy(buf[4:], body)
	_, err = w.Write(buf)
	return err
}

func readFrame(r io.Reader, want MessageType, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameSize {
		return ErrFrameTooLarge
	}

	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("proto: decode envelope: %w", err)
	}
	if env.Type != want {
		return fmt.Errorf("%w: got %q, want %q", ErrUnexpectedType, env.Type, want)
	}
	if err := json.Unmarshal(env.Payload, v); err != nil {
		return fmt.Errorf("proto: decode %s: %w", want, err)
	}
	return nil
}
