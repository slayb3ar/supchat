// db.go

package main

import (
	"database/sql"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func InitDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	return &DB{db}, nil
}

func (db *DB) CreateTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			username TEXT PRIMARY KEY,
			hashed_password TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			FOREIGN KEY (username) REFERENCES users(username)
		)`,
		`CREATE TABLE IF NOT EXISTS rooms (
			id TEXT PRIMARY KEY
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			room_id TEXT NOT NULL,
			username TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			FOREIGN KEY (room_id) REFERENCES rooms(id),
			FOREIGN KEY (username) REFERENCES users(username)
		)`,
	}

	for _, table := range tables {
		_, err := db.Exec(table)
		if err != nil {
			return fmt.Errorf("error creating table: %w", err)
		}
	}

	return nil
}

func (db *DB) GetUser(username string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT username, hashed_password FROM users WHERE username = ?", username).Scan(&user.Username, &user.HashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error querying user: %w", err)
	}
	return &user, nil
}

func (db *DB) CreateUser(username, hashedPassword string) error {
	_, err := db.Exec("INSERT INTO users (username, hashed_password) VALUES (?, ?)", username, hashedPassword)
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	return nil
}

func (db *DB) CreateSession(token, username string) error {
	_, err := db.Exec("INSERT INTO sessions (token, username) VALUES (?, ?)", token, username)
	if err != nil {
		return fmt.Errorf("error creating session: %w", err)
	}
	return nil
}

func (db *DB) GetUserFromSession(token string) (*User, error) {
	var username string
	err := db.QueryRow("SELECT username FROM sessions WHERE token = ?", token).Scan(&username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error querying session: %w", err)
	}

	return db.GetUser(username)
}

func (db *DB) CreateRoom(roomID string) error {
	_, err := db.Exec("INSERT OR IGNORE INTO rooms (id) VALUES (?)", roomID)
	if err != nil {
		return fmt.Errorf("error creating room: %w", err)
	}
	return nil
}

func (db *DB) StoreMessage(roomID, username, content, timestamp string) error {
	_, err := db.Exec("INSERT INTO messages (room_id, username, content, timestamp) VALUES (?, ?, ?, ?)",
		roomID, username, content, timestamp)
	if err != nil {
		return fmt.Errorf("error storing message: %w", err)
	}
	return nil
}

func (db *DB) GetRoomMessages(roomID string) ([]Message, error) {
	rows, err := db.Query("SELECT content, username, timestamp FROM messages WHERE room_id = ? ORDER BY timestamp", roomID)
	if err != nil {
		return nil, fmt.Errorf("error fetching room messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		err := rows.Scan(&msg.Content, &msg.User, &msg.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("error scanning message row: %w", err)
		}
		msg.Type = "message"
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows: %w", err)
	}

	return messages, nil
}

func (db *DB) GetRooms() (map[string]int, error) {
    rows, err := db.Query(`
        SELECT r.id, COUNT(DISTINCT m.username) as user_count
        FROM rooms r
        LEFT JOIN messages m ON r.id = m.room_id
        GROUP BY r.id
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    rooms := make(map[string]int)
    for rows.Next() {
        var roomID string
        var userCount int
        if err := rows.Scan(&roomID, &userCount); err != nil {
            return nil, err
        }
        rooms[roomID] = userCount
    }
    return rooms, nil
}

func (db *DB) GetUserCount() (int, error) {
    var count int
    err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
    return count, err
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
	cookie, err := r.Cookie("SessionToken")
	if err != nil {
		return nil
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	user, err := rm.db.GetUserFromSession(cookie.Value)
	if err != nil {
		log.Printf("Error getting user from session: %v", err)
		return nil
	}

	if user != nil {
		user.SessionToken = cookie.Value
	}

	return user
}


//
// Hash Password
//
func hashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

//
// Verify Password
//
func verifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
