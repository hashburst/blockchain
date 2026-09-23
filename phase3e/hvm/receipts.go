package hvm

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

func ReceiptsRoot(receipts []Receipt) string {
	var b bytes.Buffer
	putReceiptBytes(&b, []byte("HASHBURST_HVM_RECEIPTS_V1"))
	for _, r := range receipts {
		putReceiptBytes(&b, []byte(r.TxID))
		if r.Success {
			b.WriteByte(1)
		} else {
			b.WriteByte(0)
		}
		putReceiptUint64(&b, r.ComputeUsed)
		putReceiptUint64(&b, uint64(r.FeeUnits))
		putReceiptBytes(&b, []byte(r.Contract))
		putReceiptBytes(&b, r.ReturnData)
		putReceiptBytes(&b, []byte(r.RevertReason))
		putReceiptUint64(&b, uint64(len(r.Events)))
		for _, e := range r.Events {
			putReceiptBytes(&b, []byte(e.Contract))
			putReceiptBytes(&b, []byte(e.Name))
			putReceiptUint64(&b, uint64(len(e.Topics)))
			for _, t := range e.Topics {
				putReceiptBytes(&b, []byte(t))
			}
			putReceiptBytes(&b, e.Data)
		}
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:])
}

func putReceiptUint64(b *bytes.Buffer, v uint64) {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	b.Write(x[:])
}

func putReceiptBytes(b *bytes.Buffer, p []byte) {
	putReceiptUint64(b, uint64(len(p)))
	b.Write(p)
}
