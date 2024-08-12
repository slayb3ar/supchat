// Wheel.go

package main

import (
	"time"
)

//
// Message defines the structure of messages exchanged between clients.
//
type Message struct {
	Type      string `json:"type"`      // Type of message: "message", "join", "leave"
	Content   string `json:"content"`   // Content of the message
	User      string `json:"user,omitempty"` // Username of the sender (optional)
	Timestamp string `json:"timestamp"` // Timestamp of the message
}

//
// Hub maintains the set of active Clients and handles message broadcasting.
//
type Hub struct {
	Clients    map[*Client]bool   // Registered Clients
	broadcast  chan Message      // Inbound messages from the Clients
	register   chan *Client      // Register requests from the Clients
	unregister chan *Client      // Unregister requests from Clients
	history    []Message         // Chat history
}

//
// Get unique user count form hub
//
func (h *Hub) UniqueUser() int {
    uniqueUsers := make(map[string]bool)
    for client := range h.Clients {
        uniqueUsers[client.user.Username] = true
    }
    return len(uniqueUsers)
}

//
// run starts the main event loop for the Hub,
// processing register, unregister,and broadcast events.
//
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
					Timestamp: time.Now().Format("Monday 3:04PM"),
				}
				go func() { h.broadcast <- leaveMessage }()
			}

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
