package ping

import (
	"encoding/binary"
	"math/rand"
	"net"
	"strings"
	"time"
)

var offlineMessageID = []byte{
	0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe,
	0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78,
}

const (
	unconnectedPing = 0x01
	unconnectedPong = 0x1c
)

type BedrockStatus struct {
	Online  bool     `json:"online"`
	Host    string   `json:"host"`
	Port    int      `json:"port"`
	Version *Version `json:"version,omitempty"`
	Players *Players `json:"players,omitempty"`
	Motd    *Motd    `json:"motd,omitempty"`
	Latency int64    `json:"latency,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func PingBedrock(host string, port int, timeout time.Duration) *BedrockStatus {
	status := &BedrockStatus{
		Online: false,
		Host:   host,
		Port:   port,
	}

	startTime := time.Now()

	conn, err := net.DialTimeout("udp", net.JoinHostPort(host, itoa(port)), timeout)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	// Send unconnected ping
	pingPacket := createUnconnectedPing()
	if _, err := conn.Write(pingPacket); err != nil {
		status.Error = err.Error()
		return status
	}

	// Read response
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	status.Latency = time.Since(startTime).Milliseconds()
	parseBedrockResponse(buffer[:n], status)
	return status
}

func createUnconnectedPing() []byte {
	packet := make([]byte, 1+8+16+8)
	offset := 0

	packet[offset] = unconnectedPing
	offset++

	binary.BigEndian.PutUint64(packet[offset:], uint64(time.Now().UnixMilli()))
	offset += 8

	copy(packet[offset:], offlineMessageID)
	offset += 16

	binary.BigEndian.PutUint64(packet[offset:], rand.Uint64())

	return packet
}

func parseBedrockResponse(data []byte, status *BedrockStatus) {
	if len(data) < 35 || data[0] != unconnectedPong {
		status.Error = "Invalid pong response"
		return
	}

	offset := 1  // Skip packet ID
	offset += 8  // Skip timestamp
	offset += 8  // Skip server GUID
	offset += 16 // Skip magic

	if len(data) < offset+2 {
		status.Error = "Truncated response"
		return
	}

	stringLength := binary.BigEndian.Uint16(data[offset:])
	offset += 2

	if len(data) < offset+int(stringLength) {
		status.Error = "Truncated response"
		return
	}

	serverInfo := string(data[offset : offset+int(stringLength)])
	parseServerInfo(serverInfo, status)
}

func parseServerInfo(info string, status *BedrockStatus) {
	// Format: Edition;MOTD;Protocol;Version;Players;MaxPlayers;ServerID;SubMOTD;Gamemode;...
	parts := strings.Split(info, ";")

	if len(parts) < 6 {
		status.Error = "Invalid server info format"
		return
	}

	edition := parts[0]
	motdRaw := parts[1]
	protocolStr := parts[2]
	versionName := parts[3]
	playersStr := parts[4]
	maxPlayersStr := parts[5]

	protocol := 0
	for _, c := range protocolStr {
		if c >= '0' && c <= '9' {
			protocol = protocol*10 + int(c-'0')
		}
	}

	players := 0
	for _, c := range playersStr {
		if c >= '0' && c <= '9' {
			players = players*10 + int(c-'0')
		}
	}

	maxPlayers := 0
	for _, c := range maxPlayersStr {
		if c >= '0' && c <= '9' {
			maxPlayers = maxPlayers*10 + int(c-'0')
		}
	}

	status.Online = true
	status.Version = &Version{
		Name:      edition + " " + versionName,
		NameClean: versionName,
		Protocol:  protocol,
	}
	status.Players = &Players{
		Online: players,
		Max:    maxPlayers,
	}
	status.Motd = &Motd{
		Raw:   motdRaw,
		Clean: cleanMotd(motdRaw),
		Html:  motdToHtml(motdRaw),
	}
}
