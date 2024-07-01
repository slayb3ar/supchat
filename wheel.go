// Wheel.go

package main

import (
	"time"
)

// Message defines the structure of messages exchanged between clients.
type Message struct {
	Type      string `json:"type"`      // Type of message: "message", "join", "leave"
	Content   string `json:"content"`   // Content of the message
	User      string `json:"user,omitempty"` // Username of the sender (optional)
	Timestamp string `json:"timestamp"` // Timestamp of the message
}

// Hub maintains the set of active Clients and handles message broadcasting.
type Hub struct {
	Clients    map[*Client]bool   // Registered Clients
	broadcast  chan Message      // Inbound messages from the Clients
	register   chan *Client      // Register requests from the Clients
	unregister chan *Client      // Unregister requests from Clients
	history    []Message         // Chat history
}

// run starts the main event loop for the Hub, processing register, unregister,
// and broadcast events.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			// Register a new client.
			h.Clients[client] = true

			// Send chat history to the new client.
			for _, msg := range h.history {
				client.send <- msg
			}

			// Broadcast join message to all clients asynchronously.
			joinMessage := Message{
				Type:      "join",
				Content:   "has joined the chat",
				User:      client.username,
				Timestamp: time.Now().Format("Monday 3:04PM"),
			}
			go func() { h.broadcast <- joinMessage }()

		case client := <-h.unregister:
			// Unregister an existing client.
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.send)
			}

			// Broadcast leave message to all clients asynchronously.
			leaveMessage := Message{
				Type:      "leave",
				Content:   "has left the chat",
				User:      client.username,
				Timestamp: time.Now().Format("Monday 3:04PM"),
			}
			go func() { h.broadcast <- leaveMessage }()

		case message := <-h.broadcast:
			// Broadcast a message to all registered Clients.
			h.history = append(h.history, message)
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
