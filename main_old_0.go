package main

// import (
// 	"context"
// 	"encoding/json"
// 	"log"
// 	"net/http"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/coder/websocket"
// 	"github.com/gin-gonic/gin"
// )

// const (
// 	maxMessageSize = 4 * 1024
// 	writeTimeout   = 5 * time.Second
// )

// type Message struct {
// 	User string `json:"user"`
// 	Text string `json:"text"`
// }

// type Client struct {
// 	conn *websocket.Conn
// 	send chan []byte
// }

// type Hub struct {
// 	mu      sync.Mutex
// 	clients map[*Client]struct{}
// }

// func newHub() *Hub {
// 	return &Hub{clients: make(map[*Client]struct{})}
// }

// func (h *Hub) add(c *Client) {
// 	h.mu.Lock()
// 	h.clients[c] = struct{}{}
// 	h.mu.Unlock()
// }

// // remove and broadcast both hold the lock while accessing client queues,
// // so a queue cannot be closed while broadcast is sending to it.
// func (h *Hub) remove(c *Client) {
// 	h.mu.Lock()
// 	if _, ok := h.clients[c]; ok {
// 		delete(h.clients, c)
// 		close(c.send)
// 	}
// 	h.mu.Unlock()
// }

// func (h *Hub) broadcast(data []byte) {
// 	h.mu.Lock()
// 	defer h.mu.Unlock()

// 	for c := range h.clients {
// 		select {
// 		case c.send <- data:
// 		default:
// 			// This client's queue is full; remove it rather than
// 			// blocking delivery to every other client.
// 			delete(h.clients, c)
// 			close(c.send)
// 			_ = c.conn.CloseNow()
// 		}
// 	}
// }

// func (h *Hub) serveClient(conn *websocket.Conn) {
// 	client := &Client{
// 		conn: conn,
// 		send: make(chan []byte, 32),
// 	}
// 	h.add(client)
// 	defer h.remove(client)
// 	defer conn.CloseNow()

// 	// A separate writer drains this client's outgoing queue.
// 	writerDone := make(chan struct{})
// 	go func() {
// 		defer close(writerDone)

// 		for data := range client.send {
// 			ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
// 			err := conn.Write(ctx, websocket.MessageText, data)
// 			cancel()

// 			if err != nil {
// 				_ = conn.CloseNow()
// 				return
// 			}
// 		}
// 	}()

// 	for {
// 		_, data, err := conn.Read(context.Background())
// 		if err != nil {
// 			break
// 		}

// 		var msg Message
// 		if err := json.Unmarshal(data, &msg); err != nil {
// 			continue // Ignore malformed JSON.
// 		}

// 		msg.User = strings.TrimSpace(msg.User)
// 		msg.Text = strings.TrimSpace(msg.Text)
// 		if msg.User == "" || msg.Text == "" {
// 			continue
// 		}

// 		encoded, err := json.Marshal(msg)
// 		if err != nil {
// 			continue
// 		}

// 		h.broadcast(encoded)
// 	}

// 	h.remove(client)
// 	<-writerDone
// }

// func main() {
// 	hub := newHub()
// 	router := gin.Default()

// 	router.GET("/ws", func(c *gin.Context) {
// 		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
// 			// Set this to your frontend's host in production.
// 			OriginPatterns: []string{"localhost:3000"},
// 		})
// 		if err != nil {
// 			log.Printf("WebSocket accept failed: %v", err)
// 			return
// 		}

// 		conn.SetReadLimit(maxMessageSize)
// 		hub.serveClient(conn)
// 	})

// 	if err := router.Run(":8080"); err != nil && err != http.ErrServerClosed {
// 		log.Fatal(err)
// 	}
// }
