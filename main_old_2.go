package main

// import (
// 	"context"
// 	"encoding/json"
// 	"log"
// 	"net/http"
// 	"os"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/coder/websocket"
// 	"github.com/gin-gonic/gin"
// )

// const (
// 	messageQueueSize = 32
// 	maxUsernameLen   = 24
// 	maxTextLen       = 4000
// )

// var userRoster = []string{
// 	"sam",
// 	"sadia",
// 	"alex",
// 	"taylor",
// 	"jimmy",
// }

// type Client struct {
// 	name string
// 	conn *websocket.Conn
// 	send chan []byte
// }

// type Hub struct {
// 	mu      sync.Mutex
// 	users   []string
// 	clients map[string]*Client
// 	pending map[string][][]byte // recipient username -> queued messages
// }

// func newHub(users []string) *Hub {
// 	roster := append([]string(nil), users...)

// 	return &Hub{
// 		users:   roster,
// 		clients: make(map[string]*Client),
// 		pending: make(map[string][][]byte),
// 	}
// }

// func (h *Hub) isAllowedUser(name string) bool {
// 	for _, user := range h.users {
// 		if user == name {
// 			return true
// 		}
// 	}
// 	return false
// }

// func marshalJSON(value any) []byte {
// 	data, err := json.Marshal(value)
// 	if err != nil {
// 		log.Printf("JSON marshal failed: %v", err)
// 		return nil
// 	}
// 	return data
// }

// // sendLocked adds data to an online client's outgoing channel.
// // The caller must hold h.mu.
// func (h *Hub) sendLocked(name string, data []byte) bool {
// 	client, online := h.clients[name]
// 	if !online {
// 		return false
// 	}

// 	select {
// 	case client.send <- data:
// 		return true
// 	default:
// 		return false
// 	}
// }

// // queueOrSendLocked sends to an online user when possible, and otherwise
// // stores the message for delivery when they next connect.
// // The caller must hold h.mu.
// func (h *Hub) queueOrSendLocked(name string, data []byte) bool {
// 	if h.sendLocked(name, data) {
// 		return true
// 	}

// 	h.pending[name] = append(h.pending[name], data)
// 	return false
// }

// // flushPendingLocked moves queued messages into the client's outgoing channel.
// // It leaves any messages that do not fit in the channel for a later flush.
// // The caller must hold h.mu.
// func (h *Hub) flushPendingLocked(client *Client) {
// 	queue := h.pending[client.name]

// 	for len(queue) > 0 {
// 		select {
// 		case client.send <- queue[0]:
// 			queue = queue[1:]
// 		default:
// 			h.pending[client.name] = queue
// 			return
// 		}
// 	}

// 	delete(h.pending, client.name)
// }

// func (h *Hub) onlineUsersLocked() []string {
// 	online := make([]string, 0, len(h.clients))
// 	for _, user := range h.users {
// 		if _, ok := h.clients[user]; ok {
// 			online = append(online, user)
// 		}
// 	}
// 	return online
// }

// func (h *Hub) sendUserListLocked(client *Client) {
// 	statuses := make([]map[string]any, 0, len(h.users))

// 	for _, user := range h.users {
// 		_, online := h.clients[user]
// 		statuses = append(statuses, map[string]any{
// 			"name":   user,
// 			"online": online,
// 		})
// 	}

// 	data := marshalJSON(map[string]any{
// 		"type":  "users",
// 		"users": statuses,
// 	})
// 	if data != nil {
// 		select {
// 		case client.send <- data:
// 		default:
// 			log.Printf("Could not send user list to %q", client.name)
// 		}
// 	}
// }

// func (h *Hub) broadcastPresenceLocked(username string, online bool) {
// 	data := marshalJSON(map[string]any{
// 		"type":   "presence",
// 		"user":   username,
// 		"online": online,
// 	})
// 	if data == nil {
// 		return
// 	}

// 	for _, client := range h.clients {
// 		select {
// 		case client.send <- data:
// 		default:
// 			// Presence can be refreshed from the full user list on reconnect.
// 		}
// 	}
// }

// func (h *Hub) add(client *Client) bool {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	if !h.isAllowedUser(client.name) {
// 		return false
// 	}
// 	if _, exists := h.clients[client.name]; exists {
// 		return false
// 	}

// 	h.clients[client.name] = client

// 	// Send the current roster, including this user's online status.
// 	h.sendUserListLocked(client)

// 	// Send any messages queued while this user was offline.
// 	h.flushPendingLocked(client)

// 	// Tell other connected users this user is online.
// 	h.broadcastPresenceLocked(client.name, true)

// 	return true
// }

// func (h *Hub) remove(client *Client) {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	// Avoid removing a newer connection that happens to have the same name.
// 	if h.clients[client.name] != client {
// 		return
// 	}

// 	delete(h.clients, client.name)
// 	close(client.send)
// 	h.broadcastPresenceLocked(client.name, false)
// }

// // Called by the writer after each successful write, so queued messages
// // beyond the channel capacity are delivered as the channel frees up.
// func (h *Hub) flushPending(client *Client) {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	if h.clients[client.name] == client {
// 		h.flushPendingLocked(client)
// 	}
// }

// func (h *Hub) deliverMessage(from, to, text string) {
// 	message := marshalJSON(map[string]string{
// 		"type": "message",
// 		"from": from,
// 		"to":   to,
// 		"text": text,
// 	})
// 	if message == nil {
// 		return
// 	}

// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	delivered := h.queueOrSendLocked(to, message)
// 	status := "queued"
// 	if delivered {
// 		status = "delivered"
// 	}

// 	// Let the sender know whether the recipient received the message now
// 	// or will receive it when they connect.
// 	ack := marshalJSON(map[string]string{
// 		"type":   "ack",
// 		"to":     to,
// 		"status": status,
// 	})
// 	if ack != nil {
// 		h.sendLocked(from, ack)
// 	}
// }

// func (h *Hub) handleIncoming(sender string, data []byte) {
// 	var incoming struct {
// 		To   string `json:"to"`
// 		Text string `json:"text"`
// 	}

// 	if err := json.Unmarshal(data, &incoming); err != nil {
// 		h.sendError(sender, "Invalid JSON")
// 		return
// 	}

// 	incoming.To = strings.TrimSpace(incoming.To)
// 	incoming.Text = strings.TrimSpace(incoming.Text)

// 	if incoming.To == "" {
// 		h.sendError(sender, "Recipient is required")
// 		return
// 	}
// 	if incoming.Text == "" {
// 		h.sendError(sender, "Message text is required")
// 		return
// 	}
// 	if len(incoming.Text) > maxTextLen {
// 		h.sendError(sender, "Message is too long")
// 		return
// 	}

// 	if strings.EqualFold(incoming.To, "all") {
// 		h.broadcastMessage(sender, incoming.Text)
// 		return
// 	}

// 	if !h.isAllowedUser(incoming.To) {
// 		h.sendError(sender, "Unknown recipient")
// 		return
// 	}

// 	h.deliverMessage(sender, incoming.To, incoming.Text)
// }

// func (h *Hub) broadcastMessage(sender, text string) {
// 	h.mu.Lock()
// 	recipients := make([]string, 0, len(h.users))
// 	for _, user := range h.users {
// 		if user != sender {
// 			recipients = append(recipients, user)
// 		}
// 	}
// 	h.mu.Unlock()

// 	for _, recipient := range recipients {
// 		h.deliverMessage(sender, recipient, text)
// 	}
// }

// func (h *Hub) sendError(username, message string) {
// 	data := marshalJSON(map[string]string{
// 		"type": "error",
// 		"text": message,
// 	})
// 	if data == nil {
// 		return
// 	}

// 	h.mu.Lock()
// 	defer h.mu.Unlock()
// 	h.sendLocked(username, data)
// }

// func (h *Hub) serveClient(conn *websocket.Conn, username string) {
// 	client := &Client{
// 		name: username,
// 		conn: conn,
// 		send: make(chan []byte, messageQueueSize),
// 	}

// 	if !h.add(client) {
// 		_ = conn.Close(websocket.StatusPolicyViolation, "Username unavailable")
// 		return
// 	}

// 	defer h.remove(client)
// 	defer conn.CloseNow()

// 	writerDone := make(chan struct{})
// 	go func() {
// 		defer close(writerDone)

// 		for data := range client.send {
// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			err := conn.Write(ctx, websocket.MessageText, data)
// 			cancel()

// 			if err != nil {
// 				_ = conn.CloseNow()
// 				return
// 			}

// 			h.flushPending(client)
// 		}
// 	}()

// 	conn.SetReadLimit(8 * 1024)

// 	for {
// 		_, data, err := conn.Read(context.Background())
// 		if err != nil {
// 			break
// 		}

// 		h.handleIncoming(username, data)
// 	}

// 	h.remove(client)
// 	<-writerDone
// }

// func main() {
// 	hub := newHub(userRoster)
// 	router := gin.Default()

// 	router.GET("/ws", func(c *gin.Context) {
// 		username := strings.TrimSpace(c.Query("user"))

// 		if username == "" || len(username) > maxUsernameLen {
// 			c.String(http.StatusBadRequest, "Provide a valid username")
// 			return
// 		}
// 		if !hub.isAllowedUser(username) {
// 			c.String(http.StatusForbidden, "Unknown username")
// 			return
// 		}

// 		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
// 			OriginPatterns: []string{
// 				"localhost:3000",
// 				"test-gin-chat-demo.onrender.com",
// 			},
// 		})
// 		if err != nil {
// 			log.Printf("WebSocket accept failed: %v", err)
// 			return
// 		}

// 		hub.serveClient(conn, username)
// 	})

// 	port := os.Getenv("PORT")
// 	if port == "" {
// 		port = "8080"
// 	}

// 	log.Printf("Listening on :%s", port)
// 	log.Fatal(router.Run(":" + port))
// }
