package main

import (
	"encoding/binary"
	"fmt"
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

func (r *UnlimitedZoReader) Read(out []byte) (int, error) {
	prePad := (-r.Pos) % 8
	n := r.Pos / 8
	bufLeft := uint64(len(out))
	bufIndex := uint64(0)
	tempBytes := make([]byte, 8)
	fmt.Printf("bufLeft:%d\tbufIndex:%d\tr.Pos:%d\tn:%d\t", bufLeft, bufIndex, r.Pos, n)

	// prePad
	if prePad > 0 {
		if bufLeft < prePad {
			binary.LittleEndian.PutUint64(tempBytes, n)
			copy(out, tempBytes[8-prePad:8-prePad+bufLeft])
			r.Pos += bufLeft
			return len(out), nil
		}
		binary.LittleEndian.PutUint64(tempBytes, n)
		copy(out, tempBytes[8-prePad:])
		r.Pos += prePad
		bufLeft -= prePad
		bufIndex += prePad
		n += 1
		fmt.Printf("1  bufLeft:%d\tbufIndex:%d\tr.Pos:%d\tn:%d\t", bufLeft, bufIndex, r.Pos, n)
	}

	for i := uint64(0); i < bufLeft/8; i++ {
		fmt.Printf("2  bufLeft:%d\tbufIndex:%d\tr.Pos:%d\tn:%d\t", bufLeft, bufIndex, r.Pos, n)
		binary.LittleEndian.PutUint64(out[bufIndex:], n)
		r.Pos += 8
		bufLeft -= 8
		bufIndex += 8
		n += 1
	}

	// suffix
	if bufLeft > 0 {
		fmt.Printf("3  bufLeft:%d\tbufIndex:%d\tr.Pos:%d\tn:%d\t", bufLeft, bufIndex, r.Pos, n)
		binary.LittleEndian.PutUint64(tempBytes, n)
		copy(out[bufIndex:], tempBytes[0:bufLeft])
		r.Pos += bufLeft
	}

	return len(out), nil
}

//func (r *UnlimitedZoReader) Read(out []byte) (int, error) {
//	for i := range out {
//		out[i] = byte(r.Pos % 2)
//		r.Pos = r.Pos + 1
//	}
//	return len(out), nil
//}
