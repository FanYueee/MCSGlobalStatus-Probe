package protocol

import (
	"bytes"
	"errors"
	"io"
)

var ErrVarIntTooLong = errors.New("VarInt is too long")

func WriteVarInt(buf *bytes.Buffer, value int32) {
	for {
		if (value & ^0x7F) == 0 {
			buf.WriteByte(byte(value))
			return
		}
		buf.WriteByte(byte((value & 0x7F) | 0x80))
		value = int32(uint32(value) >> 7)
	}
}

func ReadVarInt(r io.Reader) (int32, error) {
	var value int32
	var position uint
	buf := make([]byte, 1)

	for {
		_, err := r.Read(buf)
		if err != nil {
			return 0, err
		}

		currentByte := buf[0]
		value |= int32(currentByte&0x7F) << position

		if (currentByte & 0x80) == 0 {
			break
		}

		position += 7
		if position >= 35 {
			return 0, ErrVarIntTooLong
		}
	}

	return value, nil
}

func VarIntSize(value int32) int {
	size := 0
	for {
		size++
		if (value & ^0x7F) == 0 {
			return size
		}
		value = int32(uint32(value) >> 7)
	}
}
