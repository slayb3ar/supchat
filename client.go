// Spoke.go

package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// Constants for WebSocket connection parameters.
const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)


// upgrader is used to upgrade HTTP connections to WebSocket connections.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client represents a WebSocket client connected to the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan Message

	// User associated with the client.
	user *User
}

// readPump pumps messages from the websocket connection to the hub.
//
// readPump runs in a per-connection goroutine. It reads messages from the
// websocket connection, processes them, and broadcasts them to all clients in
// the associated hub. The application ensures that there is at most one reader
// on a connection by executing all reads from this goroutine.
func (c *Client) readPump() {
    defer func() {
        // Unregister the client and close the connection when the function exits.
        c.hub.unregister <- c
        c.conn.Close()
    }()

    // Set maximum message size and read deadline.
    c.conn.SetReadLimit(maxMessageSize)
    c.conn.SetReadDeadline(time.Now().Add(pongWait))

    // Set pong handler to update the read deadline.
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })

    for {
        // Read a message from the websocket connection.
        _, message, err := c.conn.ReadMessage()
        if err != nil {
            // Handle unexpected close errors.
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("error: %v", err)
            }
            break
        }

        // Prepare the full message to broadcast.
        fullMessage := Message{
            Type:      "message",
            Content:   string(message),
            User:      c.user.Username,
            Timestamp: time.Now().Format("Monday 3:04PM"),
        }

        // Broadcast the message to all clients in the hub.
        c.hub.broadcast <- fullMessage
    }
}

// writePump pumps messages from the hub to the websocket connection.
//
// writePump runs in a per-connection goroutine. It sends messages from the hub
// to the websocket connection. It also handles periodic ping messages to
// maintain connection health. The application ensures that there is at most one
// writer to a connection by executing all writes from this goroutine.
func (c *Client) writePump() {
    // Create a ticker for periodic pings to the client.
    ticker := time.NewTicker(pingPeriod)
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()

    for {
        select {
        case message, ok := <-c.send:
            // Set write deadline for the message.
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if !ok {
                // Hub closed the channel.
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            // Get a writer to send a WebSocket text message.
            w, err := c.conn.NextWriter(websocket.TextMessage)
            if err != nil {
                return
            }

            // Encode the message as JSON and write it to the connection.
            if err := json.NewEncoder(w).Encode(message); err != nil {
                return
            }

            // Close the writer.
            if err := w.Close(); err != nil {
                return
            }

        case <-ticker.C:
            // Send a ping to the client to maintain connection.
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}
