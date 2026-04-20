package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mcsapi/probe/internal/ping"
)

type Task struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type Result struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Latency int64       `json:"latency,omitempty"`
}

type Heartbeat struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp,omitempty"`
}

type Client struct {
	serverURL     string
	nodeID        string
	region        string
	secret        string
	conn          *websocket.Conn
	connMu        sync.RWMutex
	writeMu       sync.Mutex
	heartbeatStop chan struct{}
}

func NewClient(serverURL, nodeID, region, secret string) *Client {
	return &Client{
		serverURL:     serverURL,
		nodeID:        nodeID,
		region:        region,
		secret:        secret,
		heartbeatStop: make(chan struct{}),
	}
}

func (c *Client) Connect() error {
	url := fmt.Sprintf("%s/v1/stream?id=%s&region=%s", c.serverURL, c.nodeID, c.region)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.secret)

	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.setConn(conn)
	log.Printf("Connected to controller: %s", c.serverURL)
	return nil
}

func (c *Client) Run() {
	go c.heartbeatLoop()

	for {
		conn := c.getConn()
		if conn == nil {
			c.reconnect()
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			c.reconnect()
			continue
		}

		var task Task
		if err := json.Unmarshal(message, &task); err != nil {
			log.Printf("Invalid task: %v", err)
			continue
		}

		go c.handleTask(task)
	}
}

func (c *Client) handleTask(task Task) {
	result := Result{
		ID: task.ID,
	}

	timeout := 5 * time.Second

	switch task.Protocol {
	case "java":
		status := ping.PingJava(task.Target, task.Port, timeout)
		result.Success = status.Online
		result.Data = status
		result.Latency = status.Latency
		if status.Error != "" {
			result.Error = status.Error
		}
	case "bedrock":
		status := ping.PingBedrock(task.Target, task.Port, timeout)
		result.Success = status.Online
		result.Data = status
		result.Latency = status.Latency
		if status.Error != "" {
			result.Error = status.Error
		}
	default:
		result.Success = false
		result.Error = "Unknown protocol: " + task.Protocol
	}

	c.sendResult(result)
}

func (c *Client) sendResult(result Result) {
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal result: %v", err)
		return
	}

	if err := c.writeMessage(data); err != nil {
		log.Printf("Failed to send result: %v", err)
	}
}

func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			heartbeat := Heartbeat{
				Type:      "heartbeat",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
			data, err := json.Marshal(heartbeat)
			if err != nil {
				log.Printf("Failed to marshal heartbeat: %v", err)
				continue
			}
			if err := c.writeMessage(data); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
			}
		case <-c.heartbeatStop:
			return
		}
	}
}

func (c *Client) reconnect() {
	for {
		log.Println("Attempting to reconnect...")
		time.Sleep(5 * time.Second)

		if err := c.Connect(); err != nil {
			log.Printf("Reconnect failed: %v", err)
			continue
		}

		log.Println("Reconnected successfully")
		return
	}
}

func (c *Client) Close() {
	select {
	case <-c.heartbeatStop:
	default:
		close(c.heartbeatStop)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) getConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *Client) setConn(conn *websocket.Conn) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil && c.conn != conn {
		c.conn.Close()
	}
	c.conn = conn
}

func (c *Client) writeMessage(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	conn := c.getConn()
	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}
