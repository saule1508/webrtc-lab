package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

type SignalingMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	To      string `json:"to"`
	From    string `json:"from"`
}

var (
	messageStore = make(map[string]chan SignalingMessage)
	mu           sync.Mutex
	port         int
)
var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo}))

func main() {
	flag.IntVar(&port, "port", 8080, "port to listen on")
	flag.Parse()
	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/receive", receiveHandler)

	logger.Info("Starting signaling server", "port", port)
	_ = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

// sendHandler receives messages and sends them to the appropriate channel
func sendHandler(w http.ResponseWriter, r *http.Request) {
	var msg SignalingMessage
	logger.Info("Send handler")
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logger.Info("message", "from", msg.From, "to", msg.To, "type", msg.Type, "content", msg.Content)

	mu.Lock()
	defer mu.Unlock()

	if ch, ok := messageStore[msg.To]; ok {
		ch <- msg
	} else {
		logger.Error("recipient not found", "recipient", msg.To)
		http.Error(w, "recipient not found", http.StatusNotFound)
	}
}

func receiveHandler(w http.ResponseWriter, r *http.Request) {
	// Get the client ID
	clientID := r.URL.Query().Get("id")
	if clientID == "" {
		http.Error(w, "missing client ID", http.StatusBadRequest)
		return
	}
	logger.Info("receive registered", "id", clientID)

	// Create a channel to send messages to this client
	mu.Lock()
	ch := make(chan SignalingMessage)
	messageStore[clientID] = ch
	mu.Unlock()

	// This function will ensure to close the channel when the request ends
	defer func() {
		mu.Lock()
		delete(messageStore, clientID) // Clean up on client disconnect
		mu.Unlock()
		close(ch)
	}()

	for {
		// Wait until a message is available to send
		select {
		case msg, ok := <-ch:
			if !ok {
				logger.Info("message channel closed for client", "id", clientID)
				return // Exit if the channel is closed
			}
			if err := json.NewEncoder(w).Encode(msg); err != nil {
				logger.Error("Failed to send message", "error", err)
				return // Exit if sending fails
			}
			// Optionally, you can flush the response writer if needed
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case <-time.After(120 * time.Second):
			// Timeout case; could return a heartbeat or simply a no-op response
			w.WriteHeader(http.StatusNoContent) // Optional: Send a no-content response
			return                              // Exit or continue based on your requirements
		}
	}
}
