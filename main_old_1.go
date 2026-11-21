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

// type ChatMessage struct {
// 	From string `json:"from"`
// 	To   string `json:"to"`
// 	Text string `json:"text"`
// }

// type Client struct {
// 	name string
// 	conn *websocket.Conn
// 	send chan []byte
// }

// type Hub struct {
// 	mu      sync.Mutex
// 	clients map[string]*Client
// }

// func newHub() *Hub {
// 	return &Hub{clients: make(map[string]*Client)}
// }

// func (h *Hub) add(c *Client) bool {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	if _, exists := h.clients[c.name]; exists {
// 		return false // Don't allow two simultaneous connections with the same name.
// 	}
// 	h.clients[c.name] = c
// 	return true
// }

// func (h *Hub) remove(c *Client) {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	if h.clients[c.name] == c {
// 		delete(h.clients, c.name)
// 		close(c.send)
// 	}
// }

// func (h *Hub) deliver(to string, data []byte) bool {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	c, exists := h.clients[to]
// 	if !exists {
// 		return false
// 	}

// 	select {
// 	case c.send <- data:
// 		return true
// 	default:
// 		// The recipient's outgoing queue is full.
// 		return false
// 	}
// }

// func (h *Hub) serveClient(conn *websocket.Conn, name string) {
// 	client := &Client{
// 		name: name,
// 		conn: conn,
// 		send: make(chan []byte, 32),
// 	}

// 	if !h.add(client) {
// 		_ = conn.CloseNow()
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
// 		}
// 	}()

// 	conn.SetReadLimit(4 * 1024)

// 	for {
// 		_, data, err := conn.Read(context.Background())
// 		if err != nil {
// 			break
// 		}

// 		var incoming struct {
// 			To   string `json:"to"`
// 			Text string `json:"text"`
// 		}
// 		if err := json.Unmarshal(data, &incoming); err != nil {
// 			continue
// 		}

// 		incoming.To = strings.TrimSpace(incoming.To)
// 		incoming.Text = strings.TrimSpace(incoming.Text)
// 		if incoming.To == "" || incoming.Text == "" {
// 			continue
// 		}

// 		outgoing, err := json.Marshal(ChatMessage{
// 			From: name,
// 			To:   incoming.To,
// 			Text: incoming.Text,
// 		})
// 		if err != nil {
// 			continue
// 		}

// 		// Deliver to the recipient.
// 		if !h.deliver(incoming.To, outgoing) {
// 			errMessage, _ := json.Marshal(map[string]string{
// 				"type": "error",
// 				"text": incoming.To + " is not connected or cannot receive messages",
// 			})
// 			h.deliver(name, errMessage)
// 			continue
// 		}

// 		// Also deliver a copy to the sender, so their chat shows their own message.
// 		if incoming.To != name {
// 			h.deliver(name, outgoing)
// 		}
// 	}

// 	h.remove(client)
// 	<-writerDone
// }

// func main() {
// 	hub := newHub()
// 	router := gin.Default()

// 	router.GET("/ws", func(c *gin.Context) {
// 		name := strings.TrimSpace(c.Query("user"))
// 		if name == "" || len(name) > 24 {
// 			c.String(http.StatusBadRequest, "Provide a username (1–24 characters)")
// 			return
// 		}

// 		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
// 			OriginPatterns: []string{"localhost:3000", "test-gin-chat-demo.onrender.com", "https://test-gin-chat-demo.onrender.com/"},
// 		})
// 		if err != nil {
// 			log.Printf("WebSocket accept failed: %v", err)
// 			return
// 		}

// 		hub.serveClient(conn, name)
// 	})

// 	port := os.Getenv("PORT")
// 	if port == "" {
// 		port = "3000" // For local testing
// 	}

// 	log.Fatal(router.Run(":8080"))
// }
