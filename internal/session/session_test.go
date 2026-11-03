package session

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func testRecord() Record {
	return Record{
		UserID:       "user-1",
		Email:        "ada@example.com",
		ServerURL:    testServerURL,
		RefreshToken: "r-1",
	}
}

// The stored format carries the identity and the refresh token, and nothing a
// restore would have to redact.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	rec := testRecord()
	blob, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for _, want := range []string{`"userId":"user-1"`, `"email":"ada@example.com"`,
		`"serverURL":"https://vault.example.com"`, `"refreshToken":"r-1"`} {
		if !bytes.Contains(blob, []byte(want)) {
			t.Fatalf("blob = %s, want it to contain %s", blob, want)
		}
	}
	got, err := Decode(blob)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != rec {
		t.Fatalf("Decode = %+v, want %+v", got, rec)
	}
}

func TestDecodeRejectsCorruptRecord(t *testing.T) {
	if _, err := Decode([]byte("not json")); err == nil {
		t.Fatal("Decode accepted a corrupt record")
	} else if !strings.Contains(err.Error(), "session: record corrupt") {
		t.Fatalf("error = %v, want the corrupt-record wording", err)
	}
}

// A record must not reach a log through %v.
func TestRecordStringRedacts(t *testing.T) {
	if got := testRecord().String(); strings.Contains(got, "r-1") {
		t.Fatalf("String = %q, want the refresh token redacted", got)
	}
}

// The in-memory store keeps one record and reports the missing state the same
// way the real store does.
func TestMemoryStore(t *testing.T) {
	s := &MemoryStore{}
	if _, err := s.Load(); !errors.Is(err, ErrNoStoredSession) {
		t.Fatalf("empty Load = %v, want ErrNoStoredSession", err)
	}
	rec := testRecord()
	if err := s.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got, err := s.Load(); err != nil || got != rec {
		t.Fatalf("Load = %+v, %v, want the saved record", got, err)
	}
	if err := s.Clear("user-2"); err != nil {
		t.Fatalf("Clear(other): %v", err)
	}
	if _, err := s.Load(); err != nil {
		t.Fatalf("clearing another account dropped the record: %v", err)
	}
	if err := s.Clear("user-1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := s.Load(); !errors.Is(err, ErrNoStoredSession) {
		t.Fatalf("Load after Clear = %v, want ErrNoStoredSession", err)
	}
	if s.Saves != 1 || len(s.Clears) != 2 {
		t.Fatalf("Saves = %d, Clears = %v, want the calls counted", s.Saves, s.Clears)
	}
}

// The no-op store is what a caller without persistence gets: nothing written,
// nothing to restore.
func TestNopStore(t *testing.T) {
	var s Store = NopStore{}
	if err := s.Save(testRecord()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.Load(); !errors.Is(err, ErrNoStoredSession) {
		t.Fatalf("Load = %v, want ErrNoStoredSession", err)
	}
	if err := s.Clear("user-1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
}
