package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	for _, payload := range [][]byte{{}, []byte(`{"a":1}`), bytes.Repeat([]byte("x"), 70_000)} {
		var buf bytes.Buffer
		if err := WriteFrame(&buf, payload); err != nil {
			t.Fatalf("WriteFrame(%d bytes): %v", len(payload), err)
		}
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame(%d bytes): %v", len(payload), err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("round trip changed the payload: got %d bytes, want %d", len(got), len(payload))
		}
		if buf.Len() != 0 {
			t.Fatalf("ReadFrame left %d bytes behind", buf.Len())
		}
	}
}

func TestReadFrameLengthIsLittleEndian(t *testing.T) {
	// A frame whose length byte order matters: only the little-endian reading
	// consumes exactly the one payload byte.
	got, err := ReadFrame(bytes.NewReader([]byte{1, 0, 0, 0, 'x', 'y'}))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if string(got) != "x" {
		t.Fatalf("got %q, want %q", got, "x")
	}
}

func TestReadFrameErrors(t *testing.T) {
	t.Run("clean EOF", func(t *testing.T) {
		if _, err := ReadFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
			t.Fatalf("got %v, want io.EOF", err)
		}
	})
	t.Run("ended in header", func(t *testing.T) {
		if _, err := ReadFrame(bytes.NewReader([]byte{1, 0})); !errors.Is(err, ErrPrematureEnd) {
			t.Fatalf("got %v, want ErrPrematureEnd", err)
		}
	})
	t.Run("ended in body", func(t *testing.T) {
		if _, err := ReadFrame(bytes.NewReader([]byte{8, 0, 0, 0, 'x'})); !errors.Is(err, ErrPrematureEnd) {
			t.Fatalf("got %v, want ErrPrematureEnd", err)
		}
	})
	t.Run("declared too long", func(t *testing.T) {
		header := make([]byte, 4)
		binary.LittleEndian.PutUint32(header, MaxMessageLength+1)
		_, err := ReadFrame(bytes.NewReader(header))
		if !errors.Is(err, ErrOversized) {
			t.Fatalf("got %v, want ErrOversized", err)
		}
	})
}

func TestWriteFrameRejectsOversizedPayload(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrame(&buf, make([]byte, MaxMessageLength+1))
	if !errors.Is(err, ErrOversized) {
		t.Fatalf("got %v, want ErrOversized", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes before rejecting the payload", buf.Len())
	}
}

func TestReadFrameOversizedMessageMentionsSize(t *testing.T) {
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, MaxMessageLength+1)
	_, err := ReadFrame(bytes.NewReader(header))
	if err == nil || !strings.Contains(err.Error(), "67108865") {
		t.Fatalf("error should name the declared size, got %v", err)
	}
}
