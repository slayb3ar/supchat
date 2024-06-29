// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"net/http"
	"html/template"
	"crypto/rand"
	"encoding/hex"
)

//
// Generate session token
//
func generateSessionToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(bytes)
}

//
// Get username from session
//
func getUsernameFromSession(rm *RoomManager, r *http.Request) string {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return ""
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	username, exists := rm.Sessions[cookie.Value]
	if !exists {
		return ""
	}

	return username
}

//
// Serves start page
//
func serveStart(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Check if POST request
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")

		if username == "" {
			http.Error(w, "Username is required", http.StatusBadRequest)
			return
		}

		rm.mu.Lock()
		defer rm.mu.Unlock()

		if _, exists := rm.Usernames[username]; exists {
			http.Error(w, "Username is already taken", http.StatusConflict)
			return
		}

		// Register the username
		sessionToken := generateSessionToken()
		rm.Usernames[username] = sessionToken
		rm.Sessions[sessionToken] = username

		http.SetCookie(w, &http.Cookie{
			Name:  "session_token",
			Value: sessionToken,
			Path:  "/",
			// Secure: true, // Uncomment this in production
			HttpOnly: true,
		})

		http.Redirect(w, r, r.Header.Get("Referer"), 302)
		return
	}
}


//
// Serves home page
//
func serveHome(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
      http.NotFound(w, r)
      return
   	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tmpl, err := template.ParseFiles("templates/home.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, rm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

//
// Serves chat room page
//
func serveChat(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	username := getUsernameFromSession(rm, r)
	log.Println("Getting username...", username)
	if username == "" {
		http.ServeFile(w, r, "templates/start.html")
		return
	}
	http.ServeFile(w, r, "templates/room.html")
}

//
// serveWs handles websocket requests from the peer.
//
func serveWs(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// get room id and assosicated hub
	roomID := r.PathValue("chatRoom")
    if roomID == "" {
    	err := "Room ID is required"
     	log.Println(err)
    	w.Write([]byte(err))
    }
    hub := rm.getHub(roomID)

    // Check username
    username := getUsernameFromSession(rm, r)
	if username == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		return
	}

   	// Upgrade the HTTP connection to a WebSocket connection.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Register user under room
	rm.Usernames[username] = roomID

	// Create a new client and register it with the hub.
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), username: username}
	client.hub.register <- client

	// Start the read and write pumps for the client.
	// Allows collection of memory referenced by the caller by doing all work in new goroutines.
	go client.writePump()
	go client.readPump()
}


func main() {
	// Setup chat room manager
	var roomManager = newRoomManager()

	// Setup MUX
	mux := http.NewServeMux()

	// Static assets
 	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

  	// Routes
  	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveHome(roomManager, w, r)
	})
  	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		serveStart(roomManager, w, r)
	})
   	mux.HandleFunc("/c/{chatRoom}", func(w http.ResponseWriter, r *http.Request) {
		serveChat(roomManager, w, r)
	})
	mux.HandleFunc("/ws/{chatRoom}", func(w http.ResponseWriter, r *http.Request) {
		serveWs(roomManager, w, r)
	})
	err := http.ListenAndServe("localhost:8000", mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
