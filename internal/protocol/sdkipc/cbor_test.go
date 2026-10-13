package sdkipc

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

// The point of separating KindArray from KindBytes is that serde encodes a
// plain Vec<u8> as an integer array and serde_bytes::ByteBuf as a byte string.
func TestEncodeIntegerArrayVsByteString(t *testing.T) {
	got := EncodeCBOR(ArrayValue(UintValue(1), UintValue(2)))
	if want := []byte{0x82, 0x01, 0x02}; !bytes.Equal(got, want) {
		t.Fatalf("integer array: got %x, want %x", got, want)
	}
	got = EncodeCBOR(BytesValue([]byte{1, 2}))
	if want := []byte{0x42, 0x01, 0x02}; !bytes.Equal(got, want) {
		t.Fatalf("byte string: got %x, want %x", got, want)
	}
}

func TestDecodeKeepsArrayAndByteStringApart(t *testing.T) {
	arr, err := DecodeCBOR([]byte{0x82, 0x01, 0x02})
	if err != nil {
		t.Fatalf("decode array: %v", err)
	}
	if arr.Kind != KindArray || len(arr.Array) != 2 {
		t.Fatalf("expected a 2-element array, got %+v", arr)
	}
	if n, _ := arr.Array[1].AsUint(); n != 2 {
		t.Fatalf("array[1] = %d, want 2", n)
	}

	bs, err := DecodeCBOR([]byte{0x42, 0x01, 0x02})
	if err != nil {
		t.Fatalf("decode byte string: %v", err)
	}
	if bs.Kind != KindBytes {
		t.Fatalf("expected a byte string, got kind %d", bs.Kind)
	}
	if b, _ := bs.AsBytes(); !bytes.Equal(b, []byte{1, 2}) {
		t.Fatalf("bytes = %x, want 0102", b)
	}
}

func TestEncodeHeadWidths(t *testing.T) {
	cases := []struct {
		v    Value
		want []byte
	}{
		{UintValue(0), []byte{0x00}},
		{UintValue(23), []byte{0x17}},
		{UintValue(24), []byte{0x18, 0x18}},
		{UintValue(255), []byte{0x18, 0xff}},
		{UintValue(256), []byte{0x19, 0x01, 0x00}},
		{UintValue(65536), []byte{0x1a, 0x00, 0x01, 0x00, 0x00}},
		{UintValue(1 << 32), []byte{0x1b, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{NegativeValue(-1), []byte{0x20}},
		{NegativeValue(-2), []byte{0x21}},
		{NullValue(), []byte{0xf6}},
		{BoolValue(true), []byte{0xf5}},
		{BoolValue(false), []byte{0xf4}},
	}
	for _, c := range cases {
		if got := EncodeCBOR(c.v); !bytes.Equal(got, c.want) {
			t.Errorf("EncodeCBOR(%+v) = %x, want %x", c.v, got, c.want)
		}
	}
}

func TestCborRoundTrip(t *testing.T) {
	value := MapValue(
		Pair{Key: TextValue("HandshakeStart"), Value: MapValue(
			Pair{Key: TextValue("ciphersuite"), Value: TextValue(NoiseCipherSuite)},
			Pair{Key: TextValue("noise_frame"), Value: ArrayValue(
				UintValue(4), UintValue(200), UintValue(70000))},
			Pair{Key: TextValue("negative"), Value: NegativeValue(-2)},
			Pair{Key: TextValue("flag"), Value: BoolValue(true)},
			Pair{Key: TextValue("nil"), Value: NullValue()},
			Pair{Key: TextValue("blob"), Value: BytesValue([]byte{1, 2, 3})},
		)},
	)
	got, err := DecodeCBOR(EncodeCBOR(value))
	if err != nil {
		t.Fatalf("round trip decode: %v", err)
	}
	if !reflect.DeepEqual(got, value) {
		t.Fatalf("round trip changed the value:\n got %+v\nwant %+v", got, value)
	}
}

func TestDecodeTruncated(t *testing.T) {
	for _, input := range [][]byte{{0x62, 0x61}, {0x19, 0x01}, {0x81}, nil} {
		if _, err := DecodeCBOR(input); !errors.Is(err, ErrCborTruncated) {
			t.Errorf("DecodeCBOR(%x) = %v, want ErrCborTruncated", input, err)
		}
	}
}

func TestLookupAndAccessors(t *testing.T) {
	m := MapValue(
		Pair{Key: TextValue("text"), Value: TextValue("v")},
		Pair{Key: TextValue("bytes"), Value: BytesValue([]byte{9})},
		Pair{Key: TextValue("num"), Value: UintValue(7)},
	)
	if s, ok := m.Lookup("text"); !ok || s.Str != "v" {
		t.Fatalf("lookup text = %+v, %v", s, ok)
	}
	if _, ok := m.Lookup("missing"); ok {
		t.Fatal("lookup of a missing key should fail")
	}
	if s, ok := m.Lookup("num"); !ok {
		t.Fatal("num not found")
	} else if n, ok := s.AsUint(); !ok || n != 7 {
		t.Fatalf("AsUint = %d, %v", n, ok)
	}
	if _, ok := m.AsText(); ok {
		t.Fatal("a map should not report as text")
	}
}
