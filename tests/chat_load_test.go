package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	baseURL          = "http://localhost:8000"
	wsURL            = "ws://localhost:8000/ws"
	concurrentUsers  = 1000 // Adjust this number for your desired load
	messagesPerUser  = 50
	maxRooms         = 1000
	testDuration     = 5 * time.Minute
)

type User struct {
	Username string
	Password string
}

type LoginResponse struct {
	SessionToken string `json:"sessionToken"`
}

func TestChatLoadTest(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	users := generateUsers(concurrentUsers)
	rooms := generateRooms(maxRooms)

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < concurrentUsers; i++ {
		wg.Add(1)
		go func(user User) {
			defer wg.Done()
			room := rooms[rand.Intn(len(rooms))]
			err := simulateUser(user, room)
			if err != nil {
				log.Printf("Error simulating user %s: %v", user.Username, err)
			}
		}(users[i])

		// Add a small delay between user connections to avoid overwhelming the server
		time.Sleep(time.Millisecond * 10)

		if time.Since(start) > testDuration {
			break
		}
	}

	wg.Wait()

	elapsed := time.Since(start)
	log.Printf("Load test completed in %v", elapsed)
}

func generateUsers(count int) []User {
	users := make([]User, count)
	for i := 0; i < count; i++ {
		users[i] = User{
			Username: fmt.Sprintf("user%d", i),
			Password: fmt.Sprintf("pass%d", i),
		}
	}
	return users
}

func generateRooms(count int) []string {
	rooms := make([]string, count)
	for i := 0; i < count; i++ {
		rooms[i] = fmt.Sprintf("room%d", i)
	}
	return rooms
}

func simulateUser(user User, room string) error {
	// Login
	sessionToken, err := login(user)
	if err != nil {
		return fmt.Errorf("login failed: %v", err)
	}

	// Connect to WebSocket
	conn, err := connectWebSocket(sessionToken, room)
	if err != nil {
		return fmt.Errorf("WebSocket connection failed: %v", err)
	}
	defer conn.Close()

	// Send and receive messages
	for i := 0; i < messagesPerUser; i++ {
		message := fmt.Sprintf("Message %d from %s", i, user.Username)
		err = sendMessage(conn, message)
		if err != nil {
			return fmt.Errorf("failed to send message: %v", err)
		}

		_, _, err = conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("failed to read message: %v", err)
		}

		time.Sleep(time.Millisecond * time.Duration(rand.Intn(1000)))
	}

	return nil
}

func login(user User) (string, error) {
	// Create a cookie jar to handle cookies
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create cookie jar: %v", err)
	}

	client := &http.Client{
		Jar: jar,
	}

	data := url.Values{}
	data.Set("username", user.Username)
	data.Set("password", user.Password)

	resp, err := client.PostForm(baseURL+"/start", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Check for SessionToken in cookies
	cookies := client.Jar.Cookies(resp.Request.URL)
	for _, cookie := range cookies {
		if cookie.Name == "SessionToken" {
			return cookie.Value, nil
		}
	}

	// If not in cookies, try to parse response body as JSON
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err == nil && loginResp.SessionToken != "" {
		return loginResp.SessionToken, nil
	}

	// If all else fails, return the response body for debugging
	return "", fmt.Errorf("session token not found. Response body: %s", string(body))
}

func connectWebSocket(sessionToken, room string) (*websocket.Conn, error) {
	header := http.Header{}
	header.Add("Cookie", fmt.Sprintf("SessionToken=%s", sessionToken))

	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("%s/%s", wsURL, room), header)
	return conn, err
}

func sendMessage(conn *websocket.Conn, message string) error {
	return conn.WriteMessage(websocket.TextMessage, []byte(message))
}
