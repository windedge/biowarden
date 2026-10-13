package sdkipc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Kind tags which arm of the CBOR union a Value holds.
type Kind uint8

const (
	KindNull Kind = iota
	KindBool
	KindUnsigned
	KindNegative
	KindText
	KindBytes
	KindArray
	KindMap
)

// Pair is one key/value entry of a KindMap value. Order is preserved because
// the Swift reference encodes maps in insertion order and the reference vectors
// depend on it.
type Pair struct {
	Key   Value
	Value Value
}

// Value is a minimal RFC 8949 CBOR value. The byte-string arm exists separately
// from the integer-array arm because serde maps Vec<u8> to an array of integers
// while serde_bytes::ByteBuf maps to a CBOR byte string; the extension tells the
// two apart when framing Noise handshakes and transport payloads.
type Value struct {
	Kind  Kind
	Bool  bool
	Uint  uint64
	Int   int64
	Str   string
	Bytes []byte
	Array []Value
	Pairs []Pair
}

// ErrCborTruncated means the input ended inside an item.
var ErrCborTruncated = errors.New("sdkipc: truncated CBOR")

// Constructors ---------------------------------------------------------------

func NullValue() Value                { return Value{Kind: KindNull} }
func BoolValue(b bool) Value          { return Value{Kind: KindBool, Bool: b} }
func UintValue(u uint64) Value        { return Value{Kind: KindUnsigned, Uint: u} }
func NegativeValue(i int64) Value     { return Value{Kind: KindNegative, Int: i} }
func TextValue(s string) Value        { return Value{Kind: KindText, Str: s} }
func BytesValue(b []byte) Value       { return Value{Kind: KindBytes, Bytes: b} }
func ArrayValue(items ...Value) Value { return Value{Kind: KindArray, Array: items} }
func MapValue(pairs ...Pair) Value    { return Value{Kind: KindMap, Pairs: pairs} }

// Accessors ------------------------------------------------------------------

// AsText returns the value only when it is a CBOR text string.
func (v Value) AsText() (string, bool) {
	if v.Kind == KindText {
		return v.Str, true
	}
	return "", false
}

// AsBytes returns the value only when it is a CBOR byte string (major type 2).
func (v Value) AsBytes() ([]byte, bool) {
	if v.Kind == KindBytes {
		return v.Bytes, true
	}
	return nil, false
}

// AsUint returns the value only when it is a non-negative integer.
func (v Value) AsUint() (uint64, bool) {
	if v.Kind == KindUnsigned {
		return v.Uint, true
	}
	return 0, false
}

// Lookup returns the value for a text key of a map.
func (v Value) Lookup(key string) (Value, bool) {
	if v.Kind != KindMap {
		return Value{}, false
	}
	for _, p := range v.Pairs {
		if k, ok := p.Key.AsText(); ok && k == key {
			return p.Value, true
		}
	}
	return Value{}, false
}

// Decoding -------------------------------------------------------------------

// DecodeCBOR decodes exactly one definite-length CBOR item. Indefinite-length
// items and tags are not produced by the SDK IPC layer, so they are rejected
// rather than guessed at.
func DecodeCBOR(data []byte) (Value, error) {
	d := decoder{data: data}
	return d.item()
}

type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) item() (Value, error) {
	if d.pos >= len(d.data) {
		return Value{}, ErrCborTruncated
	}
	initial := d.data[d.pos]
	d.pos++
	major := initial >> 5
	info := initial & 0x1f

	if major == 7 {
		switch info {
		case 20:
			return BoolValue(false), nil
		case 21:
			return BoolValue(true), nil
		case 22, 23:
			return NullValue(), nil
		default:
			return Value{}, fmt.Errorf("sdkipc: unsupported CBOR simple value %d", info)
		}
	}

	arg, err := d.argument(info)
	if err != nil {
		return Value{}, err
	}

	switch major {
	case 0:
		return UintValue(arg), nil
	case 1:
		return NegativeValue(int64(^arg)), nil
	case 2:
		b, err := d.take(arg)
		if err != nil {
			return Value{}, err
		}
		return BytesValue(b), nil
	case 3:
		b, err := d.take(arg)
		if err != nil {
			return Value{}, err
		}
		return TextValue(string(b)), nil
	case 4:
		if arg > uint64(len(d.data)) {
			return Value{}, ErrCborTruncated
		}
		items := make([]Value, 0, arg)
		for i := uint64(0); i < arg; i++ {
			item, err := d.item()
			if err != nil {
				return Value{}, err
			}
			items = append(items, item)
		}
		return ArrayValue(items...), nil
	case 5:
		if arg > uint64(len(d.data)) {
			return Value{}, ErrCborTruncated
		}
		pairs := make([]Pair, 0, arg)
		for i := uint64(0); i < arg; i++ {
			key, err := d.item()
			if err != nil {
				return Value{}, err
			}
			val, err := d.item()
			if err != nil {
				return Value{}, err
			}
			pairs = append(pairs, Pair{Key: key, Value: val})
		}
		return MapValue(pairs...), nil
	default:
		return Value{}, fmt.Errorf("sdkipc: unsupported CBOR major type %d", major)
	}
}

func (d *decoder) argument(info byte) (uint64, error) {
	switch {
	case info < 24:
		return uint64(info), nil
	case info == 24:
		b, err := d.take(1)
		if err != nil {
			return 0, err
		}
		return uint64(b[0]), nil
	case info == 25:
		b, err := d.take(2)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint16(b)), nil
	case info == 26:
		b, err := d.take(4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint32(b)), nil
	case info == 27:
		b, err := d.take(8)
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint64(b), nil
	default:
		return 0, fmt.Errorf("sdkipc: unsupported CBOR additional info %d", info)
	}
}

func (d *decoder) take(n uint64) ([]byte, error) {
	if n > uint64(len(d.data)-d.pos) {
		return nil, ErrCborTruncated
	}
	b := d.data[d.pos : d.pos+int(n)]
	d.pos += int(n)
	return b, nil
}

// Encoding -------------------------------------------------------------------

// EncodeCBOR serializes a value using the shortest head form, matching
// ciborium/serde so the reference vectors reproduce byte for byte.
func EncodeCBOR(v Value) []byte {
	return appendItem(nil, v)
}

func appendItem(out []byte, v Value) []byte {
	switch v.Kind {
	case KindNull:
		return append(out, 0xf6)
	case KindBool:
		if v.Bool {
			return append(out, 0xf5)
		}
		return append(out, 0xf4)
	case KindUnsigned:
		return appendHead(out, 0, v.Uint)
	case KindNegative:
		return appendHead(out, 1, uint64(^v.Int))
	case KindText:
		out = appendHead(out, 3, uint64(len(v.Str)))
		return append(out, v.Str...)
	case KindBytes:
		out = appendHead(out, 2, uint64(len(v.Bytes)))
		return append(out, v.Bytes...)
	case KindArray:
		out = appendHead(out, 4, uint64(len(v.Array)))
		for _, item := range v.Array {
			out = appendItem(out, item)
		}
		return out
	case KindMap:
		out = appendHead(out, 5, uint64(len(v.Pairs)))
		for _, p := range v.Pairs {
			out = appendItem(out, p.Key)
			out = appendItem(out, p.Value)
		}
		return out
	default:
		return append(out, 0xf6)
	}
}

func appendHead(out []byte, major byte, arg uint64) []byte {
	head := major << 5
	switch {
	case arg < 24:
		return append(out, head|byte(arg))
	case arg <= 0xff:
		return append(out, head|24, byte(arg))
	case arg <= 0xffff:
		return append(out, head|25, byte(arg>>8), byte(arg))
	case arg <= 0xffffffff:
		return append(out, head|26, byte(arg>>24), byte(arg>>16), byte(arg>>8), byte(arg))
	default:
		out = append(out, head|27)
		return binary.BigEndian.AppendUint64(out, arg)
	}
}
