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

type User struct {
	Username string
	Password string
	SessionToken string
}

// RoomManager manages multiple chat rooms and user sessions.
type RoomManager struct {
	Rooms     map[string]*Hub   // Maps room IDs to corresponding hubs.
	Usernames map[string]*User 	// Maps usernames to users.
	Sessions  map[string]*User 	// Maps session tokens to usernames.
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
func getUserFromSession(rm *RoomManager, r *http.Request) *User {
	// Check for session cookie
	cookie, err := r.Cookie("SessionToken")
	if err != nil {
		return nil
	}

	// Check for session via room manager
	rm.mu.Lock()
	defer rm.mu.Unlock()
	user, exists := rm.Sessions[cookie.Value]
	if !exists {
		return nil
	}

	return user
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

func serveStart(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Parse and check for username and password
	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")
	if username == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		return
	}
	if password == "" {
		http.Error(w, "Password is required", http.StatusBadRequest)
		return
	}

	// Lock the room manager to check and update user data
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if username exists
	if user, exists := rm.Usernames[username]; exists {
		// Username exists, check password
		if user.Password != password {
			http.Error(w, "Incorrect password", http.StatusUnauthorized)
			return
		}
		// Password correct, log in user
		sessionToken := generateSessionToken()
		user.SessionToken = sessionToken
		rm.Sessions[sessionToken] = user

		// Set session via cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "SessionToken",
			Value:    sessionToken,
			Path:     "/",
			// Secure: true, // Uncomment this in production
			HttpOnly: true,
		})
	} else {
		// Username doesn't exist, sign up the user
		sessionToken := generateSessionToken()
		user := &User{Username: username, Password: password, SessionToken: sessionToken}
		rm.Usernames[username] = user
		rm.Sessions[sessionToken] = user

		// Set session via cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "SessionToken",
			Value:    sessionToken,
			Path:     "/",
			// Secure: true, // Uncomment this in production
			HttpOnly: true,
		})
	}

	// Redirect to the referer or a default page
	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusFound)
	return
}

//
// Serves chat room page
//
func serveChat(rm *RoomManager, w http.ResponseWriter, r *http.Request) {
	// Check username
	user := getUserFromSession(rm, r)
	if user == nil {
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
	user := getUserFromSession(rm, r)
	if user == nil {
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

	// Create a new client and register it with the hub.
	client := &Client{
		hub: hub,
		conn: conn,
		send: make(chan Message, 256),
		user: user,
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
		Usernames: make(map[string]*User),
		Sessions: make(map[string]*User),
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
