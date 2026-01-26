package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

type Client struct {
	serverURL string
	nodeID    string
	region    string
	secret    string
	conn      *websocket.Conn
}

func NewClient(serverURL, nodeID, region, secret string) *Client {
	return &Client{
		serverURL: serverURL,
		nodeID:    nodeID,
		region:    region,
		secret:    secret,
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

	c.conn = conn
	log.Printf("Connected to controller: %s", c.serverURL)
	return nil
}

func (c *Client) Run() {
	for {
		_, message, err := c.conn.ReadMessage()
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

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("Failed to send result: %v", err)
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
	if c.conn != nil {
		c.conn.Close()
	}
}
