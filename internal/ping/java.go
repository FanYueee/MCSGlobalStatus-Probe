package ping

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"time"

	"github.com/mcsapi/probe/pkg/protocol"
)

const ProtocolVersion = 767 // 1.21.1

type JavaStatus struct {
	Online   bool        `json:"online"`
	Host     string      `json:"host"`
	Port     int         `json:"port"`
	Version  *Version    `json:"version,omitempty"`
	Players  *Players    `json:"players,omitempty"`
	Motd     *Motd       `json:"motd,omitempty"`
	Favicon  string      `json:"favicon,omitempty"`
	Latency  int64       `json:"latency,omitempty"`
	Error    string      `json:"error,omitempty"`
}

type Version struct {
	Name      string `json:"name"`
	NameClean string `json:"name_clean"`
	Protocol  int    `json:"protocol"`
}

type Players struct {
	Online int            `json:"online"`
	Max    int            `json:"max"`
	Sample []PlayerSample `json:"sample,omitempty"`
}

type PlayerSample struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type Motd struct {
	Raw   string `json:"raw"`
	Clean string `json:"clean"`
	Html  string `json:"html"`
}

func PingJava(host string, port int, timeout time.Duration) *JavaStatus {
	status := &JavaStatus{
		Online: false,
		Host:   host,
		Port:   port,
	}

	startTime := time.Now()

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, itoa(port)), timeout)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	// Send handshake
	handshake := createHandshake(host, port)
	if _, err := conn.Write(handshake); err != nil {
		status.Error = err.Error()
		return status
	}

	// Send status request
	statusRequest := protocol.WritePacket(0x00, nil)
	if _, err := conn.Write(statusRequest); err != nil {
		status.Error = err.Error()
		return status
	}

	// Read response
	response, err := readResponse(conn)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	status.Latency = time.Since(startTime).Milliseconds()
	parseJavaResponse(response, status)
	return status
}

func itoa(i int) string {
	return string([]byte{
		byte('0' + i/10000%10),
		byte('0' + i/1000%10),
		byte('0' + i/100%10),
		byte('0' + i/10%10),
		byte('0' + i%10),
	}[func() int {
		switch {
		case i >= 10000:
			return 0
		case i >= 1000:
			return 1
		case i >= 100:
			return 2
		case i >= 10:
			return 3
		default:
			return 4
		}
	}():])
}

func createHandshake(host string, port int) []byte {
	var data bytes.Buffer

	protocol.WriteVarInt(&data, ProtocolVersion)
	protocol.WriteString(&data, host)
	protocol.WriteUint16(&data, uint16(port))
	protocol.WriteVarInt(&data, 1) // Next state: Status

	return protocol.WritePacket(0x00, data.Bytes())
}

func readResponse(conn net.Conn) (string, error) {
	// Read packet length
	length, err := protocol.ReadVarInt(conn)
	if err != nil {
		return "", err
	}

	// Read packet data
	data := make([]byte, length)
	_, err = io.ReadFull(conn, data)
	if err != nil {
		return "", err
	}

	reader := bytes.NewReader(data)

	// Read packet ID
	_, err = protocol.ReadVarInt(reader)
	if err != nil {
		return "", err
	}

	// Read JSON string
	jsonStr, err := protocol.ReadString(reader)
	if err != nil {
		return "", err
	}

	return jsonStr, nil
}

type javaResponse struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample"`
	} `json:"players"`
	Description interface{} `json:"description"`
	Favicon     string      `json:"favicon"`
}

func parseJavaResponse(jsonStr string, status *JavaStatus) {
	var resp javaResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		status.Error = "Invalid JSON response"
		return
	}

	status.Online = true
	status.Version = &Version{
		Name:      resp.Version.Name,
		NameClean: cleanVersionName(resp.Version.Name),
		Protocol:  resp.Version.Protocol,
	}
	status.Players = &Players{
		Online: resp.Players.Online,
		Max:    resp.Players.Max,
	}
	for _, s := range resp.Players.Sample {
		status.Players.Sample = append(status.Players.Sample, PlayerSample{
			Name: s.Name,
			ID:   s.ID,
		})
	}
	status.Favicon = resp.Favicon

	// Parse MOTD
	motdRaw := parseDescription(resp.Description)
	status.Motd = &Motd{
		Raw:   motdRaw,
		Clean: cleanMotd(motdRaw),
		Html:  motdToHtml(motdRaw),
	}
}

func parseDescription(desc interface{}) string {
	switch v := desc.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok {
			return text
		}
	}
	return ""
}

func cleanVersionName(name string) string {
	// Simple extraction of version number
	result := ""
	inVersion := false
	for _, c := range name {
		if c >= '0' && c <= '9' {
			inVersion = true
			result += string(c)
		} else if c == '.' && inVersion {
			result += string(c)
		} else if inVersion {
			break
		}
	}
	if result == "" {
		return name
	}
	return result
}

func cleanMotd(raw string) string {
	result := ""
	skip := false
	for _, c := range raw {
		if c == '§' {
			skip = true
			continue
		}
		if skip {
			skip = false
			continue
		}
		result += string(c)
	}
	return result
}

func motdToHtml(raw string) string {
	// Simplified HTML conversion
	return cleanMotd(raw)
}
