// Main.go

package main

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"sync"
)

// RoomManager manages multiple chat rooms and user sessions.
type RoomManager struct {
	Rooms     map[string]*Hub   // Maps room IDs to corresponding hubs.
	Usernames map[string]string // Maps usernames to session tokens.
	Sessions  map[string]string // Maps session tokens to usernames.
	mu        sync.Mutex        // Mutex for safe concurrent access to maps.
}

//
// Generate session token
//
func generateSessionToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(bytes)
}

//
// Get username from session
//
func getUsernameFromSession(rm *RoomManager, r *http.Request) string {
	// Check for session cookie
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return ""
	}

	// Check for session via room manager
	rm.mu.Lock()
	defer rm.mu.Unlock()
	username, exists := rm.Sessions[cookie.Value]
	if !exists {
		return ""
	}

	return username
}

//
// Serves 404 page
//
func serve404(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "templates/404.html")
}


//
// Serves home page
//
func serveHome(rm *RoomManager, w http.ResponseWriter) {
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
// Serves start page
//
func serveStart(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Parse and check for username
	r.ParseForm()
	username := r.FormValue("username")
	if username == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		return
	}

	// Check username exists via room manager
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, exists := rm.Usernames[username]; exists {
		http.Error(w, "Username is already taken", http.StatusConflict)
		return
	}

	// Register the username / session, set via cookie
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

//
// Serves chat room page
//
func serveChat(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Check username
	username := getUsernameFromSession(rm, r)
	if username == "" {
		roomID := r.PathValue("chatRoom")
		tmpl, err := template.ParseFiles("templates/start.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = tmpl.Execute(w, roomID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}
	http.ServeFile(w, r, "templates/room.html")
}

//
// serveWs handles websocket requests from the peer.
//
func serveWs(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Check username
	username := getUsernameFromSession(rm, r)
	if username == "" {
		err := "Username is required"
		http.Error(w, err, http.StatusBadRequest)
		return
	}

	// Check room id
	roomID := r.PathValue("chatRoom")
	if roomID == "" {
		err := "Room ID is required"
		http.Error(w, err, http.StatusBadRequest)
		return
	}

	// Get or create hub
	rm.mu.Lock()
	defer rm.mu.Unlock()
	hub, exists := rm.Rooms[roomID]
	if !exists {
		hub = &Hub{
			broadcast:  make(chan Message),
			register:   make(chan *Client),
			unregister: make(chan *Client),
			Clients:    make(map[*Client]bool),
			history:    make([]Message, 0),
		}
		rm.Rooms[roomID] = hub
		go hub.run()
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
	client := &Client{
		hub: hub,
		conn: conn,
		send: make(chan Message, 256),
		username: username,
	}
	client.hub.register <- client

	// Start the read and write pumps for the client.
	// Allows collection of memory referenced by the caller by doing all work in new goroutines.
	go client.writePump()
	go client.readPump()
}

func main() {
	// Setup chat room manager
	var roomManager = &RoomManager{
		Rooms: make(map[string]*Hub),
		Usernames: make(map[string]string),
		Sessions: make(map[string]string),
	}

	// Setup MUX
	mux := http.NewServeMux()

	// Static assets
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Routes
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		serve404(w, r)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serveHome(roomManager, w)
	})
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, r *http.Request) {
		serveStart(roomManager, w, r)
	})
	mux.HandleFunc("GET /c/{chatRoom}", func(w http.ResponseWriter, r *http.Request) {
		serveChat(roomManager, w, r)
	})
	mux.HandleFunc("GET /ws/{chatRoom}", func(w http.ResponseWriter, r *http.Request) {
		serveWs(roomManager, w, r)
	})

	// Start server
	err := http.ListenAndServe("localhost:8000", mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
