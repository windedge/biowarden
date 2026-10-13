package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxMessageLength bounds one frame's payload. Chrome caps host-to-extension
// messages at 1MB; the higher ceiling here only keeps a buggy extension from
// exhausting memory, as in the reference implementation.
const MaxMessageLength = 64 << 20

var (
	// ErrPrematureEnd means the stream ended inside a frame.
	ErrPrematureEnd = errors.New("protocol: stdio stream ended mid-frame")
	// ErrOversized means a frame's declared length exceeds MaxMessageLength.
	ErrOversized = errors.New("protocol: frame too large")
)

// ReadFrame reads one native messaging frame: a 4-byte native-endian length
// followed by that many payload bytes. io.EOF before a length means the
// browser closed the stream.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrPrematureEnd
		}
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size > MaxMessageLength {
		return nil, fmt.Errorf("%w: %d bytes", ErrOversized, size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPrematureEnd, err)
	}
	return payload, nil
}

// WriteFrame writes payload as one frame. Callers serialize their own writes:
// interleaved frames would corrupt the stream.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxMessageLength {
		return fmt.Errorf("%w: %d bytes", ErrOversized, len(payload))
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("protocol: write frame header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("protocol: write frame body: %w", err)
	}
	return nil
}
