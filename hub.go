// Wheel.go

package main

import (
	"log"
	"time"
)

// Message defines the structure of messages exchanged between clients.
type Message struct {
	Type      string `json:"type"`           // Type of message: "message", "join", "leave"
	Content   string `json:"content"`        // Content of the message
	User      string `json:"user,omitempty"` // Username of the sender (optional)
	Timestamp string `json:"timestamp"`      // Timestamp of the message
	RowId     string `json:"rowid"`          // Row ID of the message
}

// Hub maintains the set of active Clients and handles message broadcasting.
type Hub struct {
	Clients    map[*Client]bool // Registered Clients
	broadcast  chan Message     // Inbound messages from the Clients
	register   chan *Client     // Register requests from the Clients
	unregister chan *Client     // Unregister requests from Clients
	history    []Message        // Chat history
	roomID     string           // Room ID
	db         *DB              // Pointer to database
}

// run starts the main event loop for the Hub,
// processing register, unregister,and broadcast events.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.Clients[client] = true

			// Send chat history to the new client
			messages, err := h.db.GetRoomMessages(h.roomID)
			if err != nil {
				log.Printf("Error fetching chat history: %v", err)
				continue
			}

			for _, msg := range messages {
				client.send <- msg
			}

			// Check if this is the first connection for this user
			// Broadcast join message if so
			isNewUser := true
			for existingClient := range h.Clients {
				if existingClient != client && existingClient.user.Username == client.user.Username {
					isNewUser = false
					break
				}
			}
			if isNewUser {
				joinMessage := Message{
					Type:      "join",
					Content:   "has joined the chat",
					User:      client.user.Username,
					Timestamp: time.Now().Format("Monday 3:04PM"),
				}
				go func() { h.broadcast <- joinMessage }()
			}

		case client := <-h.unregister:
			// Unregister an existing client.
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.send)
			}

			// Check if this was the last connection for this user
			// Broadcast leave message to all clients asynchronously.
			isLastConnection := true
			for remainingClient := range h.Clients {
				if remainingClient.user.Username == client.user.Username {
					isLastConnection = false
					break
				}
			}
			if isLastConnection {
				leaveMessage := Message{
					Type:      "leave",
					Content:   "has left the chat",
					User:      client.user.Username,
					Timestamp: time.Now().Format("Monday, 2006-01-02 15:04:05"),
				}
				go func() { h.broadcast <- leaveMessage }()
			}

		case message := <-h.broadcast:
			// Store message in the database
			err := h.db.StoreMessage(h.roomID, message.User, message.Content, message.Timestamp)
			if err != nil {
				log.Printf("Error storing message: %v", err)
			}
			for client := range h.Clients {
				select {
				case client.send <- message:
					// Successfully sent the message to the client.
				default:
					// Failed to send the message, assume client is unresponsive and clean up.
					close(client.send)
					delete(h.Clients, client)
				}
			}
		}
	}
}
