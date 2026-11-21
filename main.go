package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

const (
	messageQueueSize = 32
	maxUsernameLen   = 24
	maxTextLen       = 4000
)

var userRoster = []string{"sam", "sadia", "jim", "alex", "taylor"}

type Client struct {
	name string
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	mu      sync.Mutex
	users   []string
	clients map[string]*Client
	pending map[string][][]byte
}

func newHub(users []string) *Hub {
	return &Hub{
		users:   append([]string(nil), users...),
		clients: make(map[string]*Client),
		pending: make(map[string][][]byte),
	}
}

func (h *Hub) isAllowedUser(name string) bool {
	for _, user := range h.users {
		if user == name {
			return true
		}
	}
	return false
}

func marshalJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		log.Printf("JSON marshal failed: %v", err)
		return nil
	}
	return data
}

// All *Locked methods below require h.mu to be held by the caller.
func (h *Hub) sendLocked(name string, data []byte) bool {
	client, ok := h.clients[name]
	if !ok {
		return false
	}

	select {
	case client.send <- data:
		return true
	default:
		return false
	}
}

func (h *Hub) queueOrSendLocked(name string, data []byte) bool {
	if h.sendLocked(name, data) {
		return true
	}
	h.pending[name] = append(h.pending[name], data)
	return false
}

func (h *Hub) flushPendingLocked(client *Client) {
	queue := h.pending[client.name]

	for len(queue) > 0 {
		select {
		case client.send <- queue[0]:
			queue = queue[1:]
		default:
			h.pending[client.name] = queue
			return
		}
	}

	delete(h.pending, client.name)
}

func (h *Hub) sendUserListLocked(client *Client) {
	users := make([]map[string]any, 0, len(h.users))

	for _, username := range h.users {
		_, online := h.clients[username]
		users = append(users, map[string]any{
			"name":   username,
			"online": online,
		})
	}

	data := marshalJSON(map[string]any{
		"type":  "users",
		"users": users,
	})
	if data == nil {
		return
	}

	select {
	case client.send <- data:
	default:
		log.Printf("Could not send user list to %q", client.name)
	}
}

func (h *Hub) broadcastPresenceLocked(username string, online bool) {
	data := marshalJSON(map[string]any{
		"type":   "presence",
		"user":   username,
		"online": online,
	})
	if data == nil {
		return
	}

	for _, client := range h.clients {
		select {
		case client.send <- data:
		default:
			// The user's next refresh or reconnect provides a fresh roster.
		}
	}
}

func (h *Hub) add(client *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isAllowedUser(client.name) {
		return false
	}
	if _, exists := h.clients[client.name]; exists {
		return false
	}

	h.clients[client.name] = client
	h.sendUserListLocked(client)
	h.flushPendingLocked(client)
	h.broadcastPresenceLocked(client.name, true)
	return true
}

func (h *Hub) remove(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.name] != client {
		return
	}

	delete(h.clients, client.name)
	close(client.send)
	h.broadcastPresenceLocked(client.name, false)
}

func (h *Hub) flushPending(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.name] == client {
		h.flushPendingLocked(client)
	}
}

func (h *Hub) refreshUsers(username string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if client, ok := h.clients[username]; ok {
		h.sendUserListLocked(client)
	}
}

func (h *Hub) sendError(username, text string) {
	data := marshalJSON(map[string]string{
		"type": "error",
		"text": text,
	})
	if data == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.sendLocked(username, data)
}

func (h *Hub) deliverMessage(from, to, text string) {
	message := marshalJSON(map[string]string{
		"type": "message",
		"from": from,
		"to":   to,
		"text": text,
	})
	if message == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	delivered := h.queueOrSendLocked(to, message)
	status := "queued"
	if delivered {
		status = "delivered"
	}

	ack := marshalJSON(map[string]string{
		"type":   "ack",
		"to":     to,
		"status": status,
	})
	if ack != nil {
		h.sendLocked(from, ack)
	}
}

func (h *Hub) broadcastMessage(sender, text string) {
	h.mu.Lock()
	recipients := make([]string, 0, len(h.users))
	for _, username := range h.users {
		if username != sender {
			recipients = append(recipients, username)
		}
	}
	h.mu.Unlock()

	for _, recipient := range recipients {
		h.deliverMessage(sender, recipient, text)
	}
}

func (h *Hub) handleIncoming(sender string, data []byte) {
	var incoming struct {
		Type string `json:"type"`
		To   string `json:"to"`
		Text string `json:"text"`
	}

	if err := json.Unmarshal(data, &incoming); err != nil {
		h.sendError(sender, "Invalid JSON")
		return
	}

	if incoming.Type == "refresh" {
		h.refreshUsers(sender)
		return
	}

	incoming.To = strings.TrimSpace(incoming.To)
	incoming.Text = strings.TrimSpace(incoming.Text)

	if incoming.To == "" || incoming.Text == "" {
		h.sendError(sender, "Recipient and message text are required")
		return
	}
	if len(incoming.Text) > maxTextLen {
		h.sendError(sender, "Message is too long")
		return
	}

	if strings.EqualFold(incoming.To, "all") {
		h.broadcastMessage(sender, incoming.Text)
		return
	}

	if !h.isAllowedUser(incoming.To) {
		h.sendError(sender, "Unknown recipient")
		return
	}

	h.deliverMessage(sender, incoming.To, incoming.Text)
}

func (h *Hub) serveClient(conn *websocket.Conn, username string) {
	client := &Client{
		name: username,
		conn: conn,
		send: make(chan []byte, messageQueueSize),
	}

	if !h.add(client) {
		_ = conn.Close(websocket.StatusPolicyViolation, "Username unavailable")
		return
	}

	defer h.remove(client)
	defer conn.CloseNow()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)

		for data := range client.send {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := conn.Write(ctx, websocket.MessageText, data)
			cancel()

			if err != nil {
				_ = conn.CloseNow()
				return
			}

			h.flushPending(client)
		}
	}()

	conn.SetReadLimit(8 * 1024)

	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			break
		}
		h.handleIncoming(username, data)
	}

	h.remove(client)
	<-writerDone
}

func main() {
	hub := newHub(userRoster)
	router := gin.Default()

	router.GET("/ws", func(c *gin.Context) {
		username := strings.TrimSpace(c.Query("user"))

		if username == "" || len(username) > maxUsernameLen {
			c.String(http.StatusBadRequest, "Provide a valid username")
			return
		}
		if !hub.isAllowedUser(username) {
			c.String(http.StatusForbidden, "Unknown username")
			return
		}

		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			OriginPatterns: []string{
				"localhost:3000",
				"test-gin-chat-demo.onrender.com",
			},
		})
		if err != nil {
			log.Printf("WebSocket accept failed: %v", err)
			return
		}

		hub.serveClient(conn, username)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Chat server listening on :%s", port)
	log.Fatal(router.Run(":" + port))
}
