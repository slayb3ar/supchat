// Wheel.go
package main

import (
	"time"
)

// Defines type of message
type Message struct {
    Type    string `json:"type"`    // "message", "join", "leave"
    Content string `json:"content"`
    User    string `json:"user,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Hub maintains the set of active Clients and broadcasts messages to the Clients.
type Hub struct {
	// Registered Clients.
	Clients map[*Client]bool

	// Inbound messages from the Clients.
	broadcast chan Message

	// Register requests from the Clients.
	register chan *Client

	// Unregister requests from Clients.
	unregister chan *Client

	// Chat history
	history []Message
}

// run starts the main event loop for the Hub, processing register, unregister,
// and broadcast events.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			// Register a new client.
			h.Clients[client] = true

			// Send history to new client
			for _, msg := range h.history {
				client.send <- msg
			}

			// Broadcast join message via goroutine
			joinMessage := Message{
				Type: "join",
				Content: "has joined the chat",
				User: client.username,
				Timestamp: time.Now().Format("Monday 3:04PM"),
			}
			go func() { h.broadcast <- joinMessage }()

		case client := <-h.unregister:
			// Unregister an existing client.
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.send)
			}

			// Broadcast leave message via goroutine
   			leaveMessage := Message{
      			Type: "leave",
         		Content: "has left the chat",
           		User: client.username,
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
