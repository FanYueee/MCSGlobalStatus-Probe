package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteMessageWithoutConnectionReturnsError(t *testing.T) {
	c := NewClient("ws://127.0.0.1:3000", "local-01", "Local", "secret")

	err := c.writeMessage([]byte(`{"type":"heartbeat"}`))
	if err == nil {
		t.Fatal("expected writeMessage to fail when there is no connection")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected not connected error, got %v", err)
	}
}

func TestHeartbeatMarshalsExpectedShape(t *testing.T) {
	heartbeat := Heartbeat{
		Type:      "heartbeat",
		Timestamp: "2026-04-20T08:00:00Z",
	}

	data, err := json.Marshal(heartbeat)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	expected := `{"type":"heartbeat","timestamp":"2026-04-20T08:00:00Z"}`
	if string(data) != expected {
		t.Fatalf("unexpected heartbeat payload: got %s want %s", string(data), expected)
	}
}
