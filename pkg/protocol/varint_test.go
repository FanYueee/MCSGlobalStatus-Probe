package protocol

import (
	"bytes"
	"testing"
)

func TestWriteVarInt(t *testing.T) {
	tests := []struct {
		value    int32
		expected []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x01}},
		{255, []byte{0xff, 0x01}},
		{25565, []byte{0xdd, 0xc7, 0x01}},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		WriteVarInt(&buf, tt.value)
		if !bytes.Equal(buf.Bytes(), tt.expected) {
			t.Errorf("WriteVarInt(%d) = %v, want %v", tt.value, buf.Bytes(), tt.expected)
		}
	}
}

func TestReadVarInt(t *testing.T) {
	tests := []struct {
		input    []byte
		expected int32
	}{
		{[]byte{0x00}, 0},
		{[]byte{0x01}, 1},
		{[]byte{0x7f}, 127},
		{[]byte{0x80, 0x01}, 128},
		{[]byte{0xff, 0x01}, 255},
		{[]byte{0xdd, 0xc7, 0x01}, 25565},
	}

	for _, tt := range tests {
		reader := bytes.NewReader(tt.input)
		result, err := ReadVarInt(reader)
		if err != nil {
			t.Errorf("ReadVarInt(%v) error: %v", tt.input, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("ReadVarInt(%v) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestVarIntSize(t *testing.T) {
	tests := []struct {
		value    int32
		expected int
	}{
		{0, 1},
		{127, 1},
		{128, 2},
		{16383, 2},
		{16384, 3},
	}

	for _, tt := range tests {
		result := VarIntSize(tt.value)
		if result != tt.expected {
			t.Errorf("VarIntSize(%d) = %d, want %d", tt.value, result, tt.expected)
		}
	}
}
