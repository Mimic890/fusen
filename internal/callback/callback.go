// Package callback encodes and decodes compact inline-button payloads.
//
// Format: kind[:arg]... where kind is a short screen/action code and IDs are
// base36-encoded, e.g. "p:1a:ren". Telegram limits callback_data to 64 bytes.
// The same format is used to persist pending input state of a user.
package callback

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxLen is the Telegram limit for callback_data in bytes.
const MaxLen = 64

const sep = ":"

var (
	ErrEmpty    = errors.New("callback: empty kind")
	ErrTooLong  = errors.New("callback: data exceeds 64 bytes")
	ErrBadPart  = errors.New("callback: part contains separator or is empty")
	ErrNoArg    = errors.New("callback: missing argument")
	ErrBadIDArg = errors.New("callback: invalid id")
)

// Data is a decoded payload.
type Data struct {
	Kind string
	Args []string
}

// New builds a payload. Integer arguments are encoded as base36 IDs.
func New(kind string, args ...any) Data {
	d := Data{Kind: kind, Args: make([]string, 0, len(args))}
	for _, a := range args {
		switch v := a.(type) {
		case string:
			d.Args = append(d.Args, v)
		case int64:
			d.Args = append(d.Args, ID(v))
		case int:
			d.Args = append(d.Args, ID(int64(v)))
		default:
			panic(fmt.Sprintf("callback: unsupported arg type %T", a))
		}
	}
	return d
}

// ID encodes an integer ID in base36.
func ID(id int64) string {
	return strconv.FormatInt(id, 36)
}

// Encode serializes the payload, validating the length limit.
func (d Data) Encode() (string, error) {
	if d.Kind == "" {
		return "", ErrEmpty
	}
	parts := append([]string{d.Kind}, d.Args...)
	for _, p := range parts {
		if p == "" || strings.Contains(p, sep) {
			return "", fmt.Errorf("%w: %q", ErrBadPart, p)
		}
	}
	s := strings.Join(parts, sep)
	if len(s) > MaxLen {
		return "", fmt.Errorf("%w: %d", ErrTooLong, len(s))
	}
	return s, nil
}

// String encodes the payload and panics on invalid data. Use it for payloads
// built from trusted constants and IDs, where an error is a programming bug.
func (d Data) String() string {
	s, err := d.Encode()
	if err != nil {
		panic(err)
	}
	return s
}

// Decode parses a payload produced by Encode.
func Decode(s string) (Data, error) {
	if s == "" {
		return Data{}, ErrEmpty
	}
	if len(s) > MaxLen {
		return Data{}, ErrTooLong
	}
	parts := strings.Split(s, sep)
	for _, p := range parts {
		if p == "" {
			return Data{}, ErrBadPart
		}
	}
	return Data{Kind: parts[0], Args: parts[1:]}, nil
}

// Arg returns the i-th argument or an empty string.
func (d Data) Arg(i int) string {
	if i < 0 || i >= len(d.Args) {
		return ""
	}
	return d.Args[i]
}

// ID decodes the i-th argument as a base36 ID.
func (d Data) ID(i int) (int64, error) {
	a := d.Arg(i)
	if a == "" {
		return 0, ErrNoArg
	}
	id, err := strconv.ParseInt(a, 36, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: %q", ErrBadIDArg, a)
	}
	return id, nil
}
