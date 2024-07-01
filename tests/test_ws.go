package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/gorilla/websocket"
)

func createUser(username string) string {
	postURL := "http://127.0.0.1:8000/start"
	data := []byte(fmt.Sprintf("username=%s", username))
	req, err := http.NewRequest("POST", postURL, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err != nil {
		log.Fatalf("Error creating POST request: %v", err)
	}

	// Setup HTTP client with cookie support
	client := &http.Client{
		Jar: nil,
	}
	cookieJar, _ := cookiejar.New(nil)
	client.Jar = cookieJar
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending POST request: %v", err)
	}
	defer resp.Body.Close()

	// Debugging: Print the headers received
	log.Printf("Response Headers: %v", resp.Header)

	if cookies := resp.Cookies(); len(cookies) == 0 {
		log.Println("No cookies found in response")
	} else {
		for _, cookie := range cookies {
			log.Printf("Cookie: %s = %s", cookie.Name, cookie.Value)
			if cookie.Name == "session_token" {
				return cookie.Value
			}
		}
	}

	log.Fatalf("Session token not found in response")
	return ""
}

func loadTestWebSocket(username, roomID, sessionToken string) {
	url := fmt.Sprintf("ws://127.0.0.1:8000/ws/%s", roomID)
	headers := http.Header{}
	headers.Add("Cookie", fmt.Sprintf("session_token=%s", sessionToken))
	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
	}

	conn, _, err := dialer.Dial(url, headers)
	if err != nil {
		log.Fatalf("Error connecting to WebSocket: %v", err)
	}
	defer conn.Close()

	initialMessage := []byte(fmt.Sprintf(`{"action":"init","username":"%s","token":"%s"}`, username, sessionToken))
	if err := conn.WriteMessage(websocket.TextMessage, initialMessage); err != nil {
		log.Fatalf("Error sending initial message: %v", err)
	}

	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading message: %v", err)
				return
			}
			log.Printf("Received message: %s", message)
		}
	}()

	for {
		time.Sleep(30 * time.Second)
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			log.Printf("Error sending ping: %v", err)
			return
		}
	}
}

func main() {
	numUsers := 1000000
	numRooms := 10

	for i := 1; i <= numUsers; i++ {
		username := fmt.Sprintf("user%d_%d", i, time.Now().UnixNano())
		sessionToken := createUser(username)
		for j := 1; j <= numRooms; j++ {
			roomID := fmt.Sprintf("room%d", j)
			go loadTestWebSocket(username, roomID, sessionToken)
		}
	}

	select {}
}
