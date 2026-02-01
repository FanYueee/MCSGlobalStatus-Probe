package ping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"regexp"
	"strings"
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

// Minecraft color codes to hex
var colorCodes = map[string]string{
	"0": "#000000", "1": "#0000AA", "2": "#00AA00", "3": "#00AAAA",
	"4": "#AA0000", "5": "#AA00AA", "6": "#FFAA00", "7": "#AAAAAA",
	"8": "#555555", "9": "#5555FF", "a": "#55FF55", "b": "#55FFFF",
	"c": "#FF5555", "d": "#FF55FF", "e": "#FFFF55", "f": "#FFFFFF",
}

// Named colors from JSON format
var namedColors = map[string]string{
	"black":        "#000000",
	"dark_blue":    "#0000AA",
	"dark_green":   "#00AA00",
	"dark_aqua":    "#00AAAA",
	"dark_red":     "#AA0000",
	"dark_purple":  "#AA00AA",
	"gold":         "#FFAA00",
	"gray":         "#AAAAAA",
	"dark_gray":    "#555555",
	"blue":         "#5555FF",
	"green":        "#55FF55",
	"aqua":         "#55FFFF",
	"red":          "#FF5555",
	"light_purple": "#FF55FF",
	"yellow":       "#FFFF55",
	"white":        "#FFFFFF",
}

func PingJava(host string, port int, timeout time.Duration) *JavaStatus {
	status := &JavaStatus{
		Online: false,
		Host:   host,
		Port:   port,
	}

	// Check if host is already an IP address
	isIP := net.ParseIP(host) != nil

	// Quick validation: if not an IP and hostname looks invalid, fail fast
	if !isIP {
		// Single character or no dots = obviously invalid hostname
		if len(host) < 4 || (!strings.Contains(host, ".") && len(host) < 10) {
			status.Error = "Invalid hostname"
			return status
		}
	}

	startTime := time.Now()

	// Resolve SRV record for Minecraft (with timeout)
	connectHost := host
	connectPort := port

	if !isIP {
		// Use goroutine with timeout for DNS resolution (more reliable than context)
		type dnsResult struct {
			connectHost string
			connectPort int
			err         error
		}
		resultChan := make(chan dnsResult, 1)

		go func() {
			resolver := &net.Resolver{}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			connHost := host
			connPort := port

			// Try SRV first
			_, addrs, err := resolver.LookupSRV(ctx, "minecraft", "tcp", host)
			if err == nil && len(addrs) > 0 {
				connHost = strings.TrimSuffix(addrs[0].Target, ".")
				connPort = int(addrs[0].Port)
			}

			// If connectHost is still the original host (no SRV), we need to resolve A/AAAA
			if connHost == host {
				ips, err := resolver.LookupIPAddr(ctx, host)
				if err != nil || len(ips) == 0 {
					resultChan <- dnsResult{"", 0, fmt.Errorf("DNS resolution failed")}
					return
				}
				// Prefer IPv4
				for _, ip := range ips {
					if ipv4 := ip.IP.To4(); ipv4 != nil {
						connHost = ipv4.String()
						break
					}
				}
				if connHost == host && len(ips) > 0 {
					connHost = ips[0].IP.String()
				}
			}

			resultChan <- dnsResult{connHost, connPort, nil}
		}()

		select {
		case result := <-resultChan:
			if result.err != nil {
				status.Error = "DNS resolution failed"
				return status
			}
			connectHost = result.connectHost
			connectPort = result.connectPort
		case <-time.After(2 * time.Second):
			status.Error = "DNS resolution timeout"
			return status
		}
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(connectHost, itoa(connectPort)), timeout)
	if err != nil {
		status.Error = sanitizeError(err)
		return status
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	// Send handshake with original host (important for proxies like TCPShield)
	handshake := createHandshake(host, port)
	if _, err := conn.Write(handshake); err != nil {
		status.Error = sanitizeError(err)
		return status
	}

	// Send status request
	statusRequest := protocol.WritePacket(0x00, nil)
	if _, err := conn.Write(statusRequest); err != nil {
		status.Error = sanitizeError(err)
		return status
	}

	// Read response
	response, err := readResponse(conn)
	if err != nil {
		status.Error = sanitizeError(err)
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

	// Parse MOTD - handle both string and JSON component format
	motdRaw, motdHtml := parseDescription(resp.Description)
	status.Motd = &Motd{
		Raw:   motdRaw,
		Clean: cleanMotd(motdRaw),
		Html:  motdHtml,
	}
}

// parseDescription handles both legacy string format and JSON component format
func parseDescription(desc interface{}) (raw string, htmlOut string) {
	switch v := desc.(type) {
	case string:
		return v, motdToHtml(v)
	case map[string]interface{}:
		raw = jsonComponentToRaw(v)
		htmlOut = jsonComponentToHtml(v)
		return raw, htmlOut
	}
	return "", ""
}

// jsonComponentToRaw extracts raw text from JSON component
func jsonComponentToRaw(component map[string]interface{}) string {
	var result strings.Builder

	if text, ok := component["text"].(string); ok {
		result.WriteString(text)
	}

	if extra, ok := component["extra"].([]interface{}); ok {
		for _, e := range extra {
			switch ev := e.(type) {
			case string:
				result.WriteString(ev)
			case map[string]interface{}:
				result.WriteString(jsonComponentToRaw(ev))
			}
		}
	}

	return result.String()
}

// jsonComponentToHtml converts JSON component to HTML with colors and formatting
func jsonComponentToHtml(component map[string]interface{}) string {
	var result strings.Builder
	var styles []string

	// Handle color
	if color, ok := component["color"].(string); ok {
		colorHex := color
		if named, exists := namedColors[strings.ToLower(color)]; exists {
			colorHex = named
		}
		if strings.HasPrefix(colorHex, "#") {
			styles = append(styles, fmt.Sprintf("color: %s", colorHex))
		}
	}

	// Handle formatting
	if bold, ok := component["bold"].(bool); ok && bold {
		styles = append(styles, "font-weight: bold")
	}
	if italic, ok := component["italic"].(bool); ok && italic {
		styles = append(styles, "font-style: italic")
	}
	if underlined, ok := component["underlined"].(bool); ok && underlined {
		styles = append(styles, "text-decoration: underline")
	}
	if strikethrough, ok := component["strikethrough"].(bool); ok && strikethrough {
		styles = append(styles, "text-decoration: line-through")
	}

	// Build HTML
	hasStyle := len(styles) > 0
	if hasStyle {
		result.WriteString(fmt.Sprintf(`<span style="%s">`, strings.Join(styles, "; ")))
	}

	// Add text content
	if text, ok := component["text"].(string); ok {
		result.WriteString(escapeHtml(text))
	}

	// Handle extra components
	if extra, ok := component["extra"].([]interface{}); ok {
		for _, e := range extra {
			switch ev := e.(type) {
			case string:
				result.WriteString(escapeHtml(ev))
			case map[string]interface{}:
				result.WriteString(jsonComponentToHtml(ev))
			}
		}
	}

	if hasStyle {
		result.WriteString("</span>")
	}

	return result.String()
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

var colorCodeRegex = regexp.MustCompile(`§[0-9a-fk-or]`)

func cleanMotd(raw string) string {
	return colorCodeRegex.ReplaceAllString(raw, "")
}

// motdToHtml converts legacy MOTD format (with § color codes) to HTML
func motdToHtml(raw string) string {
	var result strings.Builder
	var currentColor string
	bold := false
	italic := false
	underline := false
	strikethrough := false

	var buffer strings.Builder

	flush := func() {
		if buffer.Len() > 0 {
			var styles []string
			if currentColor != "" {
				styles = append(styles, fmt.Sprintf("color: %s", currentColor))
			}
			if bold {
				styles = append(styles, "font-weight: bold")
			}
			if italic {
				styles = append(styles, "font-style: italic")
			}
			if underline {
				styles = append(styles, "text-decoration: underline")
			}
			if strikethrough {
				styles = append(styles, "text-decoration: line-through")
			}

			escaped := escapeHtml(buffer.String())
			if len(styles) > 0 {
				result.WriteString(fmt.Sprintf(`<span style="%s">%s</span>`, strings.Join(styles, "; "), escaped))
			} else {
				result.WriteString(escaped)
			}
			buffer.Reset()
		}
	}

	chars := []rune(raw)
	for i := 0; i < len(chars); i++ {
		if chars[i] == '§' && i+1 < len(chars) {
			flush()
			code := strings.ToLower(string(chars[i+1]))
			i++

			if hex, ok := colorCodes[code]; ok {
				currentColor = hex
				bold = false
				italic = false
				underline = false
				strikethrough = false
			} else if code == "r" {
				currentColor = ""
				bold = false
				italic = false
				underline = false
				strikethrough = false
			} else if code == "l" {
				bold = true
			} else if code == "o" {
				italic = true
			} else if code == "n" {
				underline = true
			} else if code == "m" {
				strikethrough = true
			}
		} else {
			buffer.WriteRune(chars[i])
		}
	}
	flush()

	return result.String()
}

func escapeHtml(text string) string {
	escaped := html.EscapeString(text)
	// Convert newlines to <br>
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return escaped
}
