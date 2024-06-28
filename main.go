// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"net/http"
	"html/template"
)
//
// Serves start page
//
func serveStart(w http.ResponseWriter, r *http.Request) {
	// Check if GET request
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.ServeFile(w, r, "templates/start.html")
}

//
// Serves home page
//
func serveHome(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Check if GET request
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
	// Check username exists
    username := ""
    cookies := r.Cookies()
    for _, c := range cookies {
        if c.Name == "username" {
            username = c.Value
        }
    }
	if username == "" {
    	err := "Username is required"
     	log.Println(err)
     	http.ServeFile(w, r, "templates/start.html")
    }

    // OK
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

    // Check username exists
    username := ""
    cookies := r.Cookies()
    for _, c := range cookies {
        if c.Name == "username" {
            username = c.Value
        }
    }
    if username == "" {
    	err := "Username is required"
     	log.Println(err)
     	w.Write([]byte(err))
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
 	mux.Handle("/design/", http.StripPrefix("/design/", http.FileServer(http.Dir("design"))))

  	// Routes
  	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveHome(roomManager, w, r)
	})
   	mux.HandleFunc("/{chatRoom}", func(w http.ResponseWriter, r *http.Request) {
		serveChat(roomManager, w, r)
	})
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		serveStart(w, r)
	})
	mux.HandleFunc("/ws/{chatRoom}", func(w http.ResponseWriter, r *http.Request) {
		serveWs(roomManager, w, r)
	})
	err := http.ListenAndServe("localhost:8000", mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
