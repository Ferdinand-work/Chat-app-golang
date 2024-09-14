package main

import (
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn     *websocket.Conn
	username string
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	registeredUsers = make(map[string]bool) // Keeps track of registered users
	clients         = make(map[string]*Client)
	mu              sync.Mutex
)

func main() {
	router := gin.Default()

	// Serve static files
	router.Static("/static", "./static")

	// Serve the main HTML file
	router.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	// Registration endpoint
	router.POST("/register", registerUser)

	// WebSocket endpoint
	router.GET("/ws", handleWebSocket)

	// Start the server
	log.Println("Server started on :8080")
	router.Run(":8080")
}

func registerUser(c *gin.Context) {
	username := c.PostForm("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username is required"})
		return
	}

	mu.Lock()
	_, exists := registeredUsers[username]
	if exists {
		mu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		return
	}
	registeredUsers[username] = true
	mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Registration successful"})
}

func handleWebSocket(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		http.Error(c.Writer, "Username is required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	_, registered := registeredUsers[username]
	if !registered {
		mu.Unlock()
		http.Error(c.Writer, "User not registered", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Error while upgrading connection:", err)
		http.Error(c.Writer, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn, username: username}

	clients[username] = client
	mu.Unlock()

	log.Printf("User %s connected", username)

	defer func() {
		mu.Lock()
		delete(clients, username)
		mu.Unlock()
		log.Printf("User %s disconnected", username)
	}()

	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			break
		}

		messageContent := string(msg)
		if len(messageContent) == 0 {
			continue
		}

		parts := strings.SplitN(messageContent, ":", 2)
		if len(parts) < 2 {
			continue
		}

		recipientUsername := parts[0]
		message := parts[1]

		mu.Lock()
		recipientClient, exists := clients[recipientUsername]
		if exists {
			err := recipientClient.conn.WriteMessage(messageType, []byte(username+": "+message))
			if err != nil {
				log.Println("Error sending message:", err)
				recipientClient.conn.Close()
				delete(clients, recipientUsername)
			}
		}
		mu.Unlock()
	}
}
