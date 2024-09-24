// Main.go

package main

import (
 	"flag"
	"html/template"
	"log"
	"net/http"
	"time"
	"sync"
)

// Logger is a middleware that logs HTTP requests
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a ResponseRecorder to capture the status code
		rr := &ResponseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rr, r)

		duration := time.Since(start)
		log.Printf("Method: %s, Route: %s, Status: %d, Duration: %v\n",
			r.Method, r.URL.Path, rr.statusCode, duration)
	})
}

// ResponseRecorder wraps the ResponseWriter to capture the status code
type ResponseRecorder struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code
func (rr *ResponseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// User struct
type User struct {
	Username string
	HashedPassword string
	SessionToken string
}

// RoomManager manages multiple chat rooms and user sessions.
type RoomManager struct {
	Rooms     map[string]*Hub   // Maps room IDs to corresponding hubs.
	Usernames map[string]*User 	// Maps usernames to users.
	Sessions  map[string]*User 	// Maps session tokens to usernames.
	mu        sync.Mutex        // Mutex for safe concurrent access to maps.
	db        *DB 				// Pointer to database
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
    type TemplateData struct {
        Rooms     map[string]int
        UserCount int
    }

    rooms, err := rm.db.GetRooms()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    userCount, err := rm.db.GetUserCount()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    data := TemplateData{
        Rooms:     rooms,
        UserCount: userCount,
    }

    tmpl, err := template.ParseFiles("templates/home.html")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    err = tmpl.Execute(w, data)
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

	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, err := rm.db.GetUser(username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User doesn't exist, create new user
		hashedPassword, err := hashPassword(password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = rm.db.CreateUser(username, hashedPassword)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		user = &User{Username: username, HashedPassword: hashedPassword}
	} else {
		// User exists, verify password
		if !verifyPassword(user.HashedPassword, password) {
			http.Error(w, "Incorrect password", http.StatusUnauthorized)
			return
		}
	}

	// Create session
	sessionToken := generateSessionToken()
	err = rm.db.CreateSession(sessionToken, username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user.SessionToken = sessionToken
	rm.Sessions[sessionToken] = user

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "SessionToken",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
	})

	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusFound)
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
	hub, exists := rm.Rooms[roomID]
	if !exists {
		err := rm.db.CreateRoom(roomID)
		if err != nil {
			log.Printf("Error creating room: %v", err)
			rm.mu.Unlock()
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		hub = &Hub{
			broadcast:  make(chan Message),
			register:   make(chan *Client),
			unregister: make(chan *Client),
			Clients:    make(map[*Client]bool),
			roomID:     roomID,
			db:         rm.db,
		}
		rm.Rooms[roomID] = hub
		go hub.run()
	}
	rm.mu.Unlock()

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
	// Init database and create tables
	db, err := InitDB("chat.db", false)
	if err != nil {
		log.Fatal("Database initialization failed: ", err)
	}
	defer db.Close()
	err = db.CreateTables()
	if err != nil {
		log.Fatal("Table creation failed: ", err)
	}
	var roomManager = &RoomManager{
		Rooms:     make(map[string]*Hub),
		Usernames: make(map[string]*User),
		Sessions:  make(map[string]*User),
		db:        db,
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
    port := flag.String("port", "8000", "specify the port to listen on")
    flag.Parse()
    err = http.ListenAndServe(":" + *port, mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
