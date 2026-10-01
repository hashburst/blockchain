package blockchain

import (
	"reflect"
	"testing"
	"time"
)

// Populate every field so adding a new protocol field without updating the codec
// breaks round-trip tests. Reflection is used ONLY in this test fixture.
func fillLedgerFixture(v reflect.Value) {
	if v.Type() == reflect.TypeOf(time.Time{}) {
		v.Set(reflect.ValueOf(time.Unix(1700000000, 12345).UTC()))
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillLedgerFixture(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillLedgerFixture(v.Field(i))
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fillLedgerFixture(v.Index(0))
	case reflect.String:
		v.SetString("test-field")
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(7)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(3)
	case reflect.Float64:
		v.SetFloat(1.25)
	default:
		panic("extend fixture for new field kind")
	}
}
func TestLedgerCodecAllBlockFields(t *testing.T) {
	for _, version := range []uint16{0, 1, 2, 3, 4} {
		var b Block
		fillLedgerFixture(reflect.ValueOf(&b).Elem())
		b.Version = version
		b.Hash = b.GenerateHash()
		data, e := EncodeLedgerBlock(nil, &b)
		if e != nil {
			t.Fatal(e)
		}
		var restored Block
		if e = DecodeLedgerBlockInto(data, &restored); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(b, restored) {
			t.Fatalf("field lost at version %d: %#v != %#v", version, b, restored)
		}
		if restored.GenerateHash() != b.Hash {
			t.Fatal("consensus hash changed")
		}
		for i := 0; i < len(data); i++ {
			if e = DecodeLedgerBlockInto(data[:i], &restored); e == nil {
				t.Fatalf("truncated record accepted at %d", i)
			}
		}
	}
}
func TestLedgerCodecNilEmpty(t *testing.T) {
	for _, b := range []*Block{{}, {Transactions: []*Transaction{nil}, EthereumTransactions: [][]byte{nil, {}}, Timestamp: time.Unix(0, 0).UTC()}} {
		data, e := EncodeLedgerBlock(nil, b)
		if e != nil {
			t.Fatal(e)
		}
		var out Block
		if e = DecodeLedgerBlockInto(data, &out); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(*b, out) {
			t.Fatal("nil/empty semantics lost")
		}
	}
}
func FuzzLedgerCodec(f *testing.F) {
	b, _ := EncodeLedgerBlock(nil, NewGenesisBlock())
	f.Add(b)
	f.Fuzz(func(t *testing.T, data []byte) {
		var out Block
		if DecodeLedgerBlockInto(data, &out) == nil {
			if _, e := EncodeLedgerBlock(nil, &out); e != nil {
				t.Fatal(e)
			}
		}
	})
}
func BenchmarkLedgerBlockCodec(b *testing.B) {
	var block Block
	fillLedgerFixture(reflect.ValueOf(&block).Elem())
	data, _ := EncodeLedgerBlock(nil, &block)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if e := WithLedgerBlock(data, func(*Block) error { return nil }); e != nil {
			b.Fatal(e)
		}
	}
}
