package ledger

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const FrameHeader = 16
const MaxRecord = 16 << 20

var frameCRC = crc32.MakeTable(crc32.Castagnoli)

// Frame is a storage envelope, not the consensus hash or an authentication proof.
func AppendFrame(dst, payload []byte) ([]byte, error) {
	if len(payload) > MaxRecord {
		return nil, errors.New("record too large")
	}
	at := len(dst)
	dst = append(dst, make([]byte, FrameHeader)...)
	copy(dst[at:], "HBX2")
	binary.LittleEndian.PutUint32(dst[at+4:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(dst[at+8:], crc32.Checksum(payload, frameCRC))
	return append(dst, payload...), nil
}
func FramePayload(frame []byte) ([]byte, error) {
	if len(frame) < FrameHeader || string(frame[:4]) != "HBX2" || binary.LittleEndian.Uint32(frame[12:16]) != 0 {
		return nil, errors.New("invalid frame header")
	}
	n := binary.LittleEndian.Uint32(frame[4:8])
	if n > MaxRecord || int(n) != len(frame)-FrameHeader {
		return nil, errors.New("invalid frame length")
	}
	b := frame[FrameHeader:]
	if crc32.Checksum(b, frameCRC) != binary.LittleEndian.Uint32(frame[8:12]) {
		return nil, errors.New("frame checksum mismatch")
	}
	return b, nil
}

// ReadFrame reuses caller scratch. Pass a bufio.Reader for sequential scans, or
// io.NewSectionReader(file, offset, length) for seek-free, concurrent indexed reads.
func ReadFrame(r io.Reader, scratch []byte) ([]byte, error) {
	var h [FrameHeader]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return nil, e
	}
	if string(h[:4]) != "HBX2" {
		return nil, errors.New("not an HBX2 frame; legacy file requires explicit migration")
	}
	n := binary.LittleEndian.Uint32(h[4:8])
	if n > MaxRecord {
		return nil, errors.New("record too large")
	}
	size := FrameHeader + int(n)
	if cap(scratch) < size {
		scratch = make([]byte, size)
	} else {
		scratch = scratch[:size]
	}
	copy(scratch, h[:])
	if _, e := io.ReadFull(r, scratch[FrameHeader:]); e != nil {
		return nil, e
	}
	if _, e := FramePayload(scratch); e != nil {
		return nil, e
	}
	return scratch, nil
}
