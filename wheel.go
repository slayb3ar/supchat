// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
)

// Hub maintains the set of active Clients and broadcasts messages to the Clients.
type Hub struct {
	// Registered Clients.
	Clients map[*Client]bool

	// Inbound messages from the Clients.
	broadcast chan []byte

	// Register requests from the Clients.
	register chan *Client

	// Unregister requests from Clients.
	unregister chan *Client

	// Chat history
 	history    []string
}

// newHub initializes a new Hub.
func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		Clients:    make(map[*Client]bool),
		history: 	make([]string, 0),
	}
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
                client.send <- []byte(msg)
            }

		case client := <-h.unregister:
			// Unregister an existing client.
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			// Broadcast a message to all registered Clients.
			h.history = append(h.history, string(message))
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
