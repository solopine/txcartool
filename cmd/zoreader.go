package main

import (
	"io"
)

type ZoReader struct {
	*io.LimitedReader
}

func NewZoReader(size uint64) io.Reader {
	return &ZoReader{(io.LimitReader(NewUnlimitedZoReader(), int64(size))).(*io.LimitedReader)}
}

// TODO: extract this to someplace where it can be shared with lotus
type UnlimitedZoReader struct {
	Pos       uint64
	PosBytes  []byte
	ByteIndex uint
}

func NewUnlimitedZoReader() io.Reader {
	return &UnlimitedZoReader{
		Pos:       0,
		PosBytes:  make([]byte, 8),
		ByteIndex: 0,
	}
}

//func (r *UnlimitedZoReader) Read(out []byte) (int, error) {
//	for i := range out {
//		out[i] = r.PosBytes[r.ByteIndex]
//		if r.ByteIndex < 7 {
//			r.ByteIndex = r.ByteIndex + 1
//		} else {
//			r.ByteIndex = 0
//			r.Pos = r.Pos + 1
//			binary.LittleEndian.PutUint64(r.PosBytes, r.Pos)
//		}
//	}
//	return len(out), nil
//}

func (r *UnlimitedZoReader) Read(out []byte) (int, error) {
	for i := range out {
		out[i] = byte(r.Pos % 2)
		r.Pos = r.Pos + 1
	}
	return len(out), nil
}
