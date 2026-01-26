package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
)

func WriteString(buf *bytes.Buffer, s string) {
	data := []byte(s)
	WriteVarInt(buf, int32(len(data)))
	buf.Write(data)
}

func ReadString(r io.Reader) (string, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}

	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func WriteUint16(buf *bytes.Buffer, value uint16) {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, value)
	buf.Write(b)
}

func WritePacket(packetID int32, data []byte) []byte {
	var buf bytes.Buffer

	// Packet ID
	var idBuf bytes.Buffer
	WriteVarInt(&idBuf, packetID)

	// Total length = packet ID length + data length
	WriteVarInt(&buf, int32(idBuf.Len()+len(data)))
	buf.Write(idBuf.Bytes())
	buf.Write(data)

	return buf.Bytes()
}
