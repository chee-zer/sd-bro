package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/vertexai/genai"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type config struct {
	port         string
	apiKey       string
	clientConfig ClientConfig
}

type ClientConfig struct {
	Project  string
	Location string
}

type ChatSession struct {
	ID          string
	ChatHistory []*genai.Content // context for llm, i think this is also handled by the generateWithStream Update: switched to chat instead of just gen
	// SystemInstruction  string       //added in the initial prompt for now, dont know if it'll work properly, this is a fallback
	ArticleURL       string // Stored for reference, directly sent to LLM
	StartTime        time.Time
	TimeLimitSeconds int
	IsActive         bool
	LastActivityTime time.Time
}

var (
	chatSessions  = make(map[string]*ChatSession) //in memory cuz hackathon
	sessionsMutex sync.RWMutex
)

func main() {
	_ = godotenv.Load()
	//logging with fatal here doesnt let me deploy
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	projectID := os.Getenv("PROJECT_ID")
	location := os.Getenv("LOCATION")

	clientConfig := ClientConfig{
		Project:  projectID,
		Location: location,
	}
	cfg := config{
		port:         port,
		apiKey:       apiKey,
		clientConfig: clientConfig,
	}

	mux := http.NewServeMux()
	var s http.Server
	s.Addr = ":" + cfg.port
	s.Handler = mux

	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("POST /start", cfg.startChatHandler)
	mux.HandleFunc("POST /chat/{sessionId}", cfg.chatHandler)

	log.Printf("server listening on port: %v ...", s.Addr)
	log.Fatal(s.ListenAndServe())
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("OK"))
}

type StartChatRequest struct {
	ArticleLink      string `json:"articleLink"`
	TimeLimitSeconds int    `json:"timeLimitSeconds"`
}

// type StartChatResponse struct {
// 	SessionID string `json:"sessionId"`
// 	Message   string `json:"message"`
// 	Error     string `json:"error,omitempty"`
// }

func (cfg *config) startChatHandler(w http.ResponseWriter, r *http.Request) {

	var req StartChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ArticleLink == "" || urlNotlValid(req.ArticleLink) {
		http.Error(w, "A valid 'articleLink' is required", http.StatusBadRequest)
		return
	}

	if req.TimeLimitSeconds <= 0 {
		req.TimeLimitSeconds = 300
	}

	sessionID := uuid.New().String()
	newSession := &ChatSession{
		ID:               sessionID,
		StartTime:        time.Now(),
		TimeLimitSeconds: req.TimeLimitSeconds,
		IsActive:         true,
		LastActivityTime: time.Now(),
	}
	sessionsMutex.Lock()
	chatSessions[sessionID] = newSession
	sessionsMutex.Unlock()

	log.Println("New session started: ", sessionID)

	//cuz streaming the first response directly when chat is created, not ideal but eh
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "event: session_id\ndata: %s\n\n", sessionID)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Start streaming the LLM's first response.
	cfg.generateAndStreamResponse(w, r, newSession)
}

// type SendMessageResponse struct {
// 	LLMResponse string `json:"llmResponse"`
// 	Message     string `json:"message"`
// 	Error       string `json:"error,omitempty"`
// }

type ChatRequest struct {
	UserMessage string `json:"userMessage"`
}

func (cfg *config) chatHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		http.Error(w, "Missing session ID in URL path", http.StatusBadRequest)
		return
	}

	sessionsMutex.RLock()
	session, ok := chatSessions[sessionID]
	sessionsMutex.RUnlock()

	if !ok || !session.IsActive {
		http.Error(w, "Session not found or has expired", http.StatusNotFound)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserMessage == "" {
		http.Error(w, "User message cannot be empty", http.StatusBadRequest)
		return
	}

	session.ChatHistory = append(session.ChatHistory, &genai.Content{
		Parts: []genai.Part{genai.Text(req.UserMessage)},
		Role:  "user",
	})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	cfg.generateAndStreamResponse(w, r, session)
}

// func resWithError(w http.ResponseWriter, code int, msg string) {
// 	if err := resWithJson(w, code, struct{ Error string }{Error: msg}); err != nil {
// 		http.Error(w, err.Error(), code)
// 	}
// }

// func resWithJson(w http.ResponseWriter, code int, payload any) error {
// 	dat, err := json.Marshal(payload)
// 	if err != nil {
// 		return err
// 	}
// 	w.WriteHeader(code)
// 	w.Write(dat)
// 	return nil
// }
