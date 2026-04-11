package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	Version         = 1
	MaxContentBytes = 320
)

var (
	ErrInvalidVersion    = errors.New("invalid version")
	ErrEmptyContent      = errors.New("empty content")
	ErrContentTooLarge   = errors.New("content exceeds 320 bytes")
	ErrInvalidUTF8       = errors.New("content is not valid UTF-8")
	ErrHashMismatch      = errors.New("message hash mismatch")
	ErrInvalidHashLength = errors.New("invalid hash length")
)

// Message is the logical message object carried across the wire and persisted locally.
type Message struct {
	V uint8  `json:"v"`
	T uint64 `json:"t"`
	C string `json:"c"`
	H string `json:"h"`
	P uint32 `json:"p"`
}

func NewMessage(content string) (*Message, error) {
	msg := &Message{
		V: Version,
		T: uint64(time.Now().Unix()),
		C: normalizeContent(content),
	}
	if err := msg.ValidateContent(); err != nil {
		return nil, err
	}
	msg.H = msg.ComputeHash()
	return msg, nil
}

func (m *Message) ValidateContent() error {
	if m.V != Version {
		return ErrInvalidVersion
	}
	m.C = normalizeContent(m.C)
	if m.C == "" {
		return ErrEmptyContent
	}
	if !utf8.ValidString(m.C) {
		return ErrInvalidUTF8
	}
	if len([]byte(m.C)) > MaxContentBytes {
		return ErrContentTooLarge
	}
	return nil
}

func (m *Message) EncodeBody() ([]byte, error) {
	if err := m.ValidateContent(); err != nil {
		return nil, err
	}
	contentBytes := []byte(m.C)
	body := make([]byte, 1+8+2+len(contentBytes))
	body[0] = m.V
	binary.BigEndian.PutUint64(body[1:9], m.T)
	binary.BigEndian.PutUint16(body[9:11], uint16(len(contentBytes)))
	copy(body[11:], contentBytes)
	return body, nil
}

func (m *Message) ComputeHash() string {
	body, err := m.EncodeBody()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (m *Message) VerifyHash() error {
	if len(m.H) != 64 {
		return ErrInvalidHashLength
	}
	if m.ComputeHash() != m.H {
		return ErrHashMismatch
	}
	return nil
}

func DecodeHashHex(hashHex string) ([]byte, error) {
	if len(hashHex) != 64 {
		return nil, ErrInvalidHashLength
	}
	return hex.DecodeString(hashHex)
}

func normalizeContent(content string) string {
	return norm.NFC.String(content)
}
