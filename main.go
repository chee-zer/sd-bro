package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	genai "google.golang.org/genai"
)

type config struct {
	port         string
	apiKey       string
	clientConfig *genai.ClientConfig
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
	err := godotenv.Load()
	if err != nil {
		log.Fatal("couldn't get enivornment variables: ", err)
	}

	apiKey := os.Getenv("API_KEY")
	port := os.Getenv("PORT")
	projectID := os.Getenv("PROJECT_ID")
	location := os.Getenv("LOCATION")

	clientConfig := &genai.ClientConfig{
		Project:  projectID,
		Location: location,
		Backend:  genai.BackendVertexAI,
		HTTPOptions: genai.HTTPOptions{
			APIVersion: "v1",
		},
	}
	cfg := config{
		port:         port,
		apiKey:       apiKey,
		clientConfig: clientConfig,
	}

	mux := http.NewServeMux()
	var s http.Server
	s.Addr = cfg.port
	s.Handler = mux

	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("POST /start", cfg.startChatHandler)

	log.Printf("server started on port: %v ...", s.Addr)
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

type StartChatResponse struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

func (cfg *config) startChatHandler(w http.ResponseWriter, r *http.Request) {

	var req StartChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resWithError(w, 400, "invalid request")
		return
	}
	if req.ArticleLink == "" || urlNotlValid(req.ArticleLink) {
		resWithError(w, 400, "please provide a valid article link")
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

	initialPrompt := fmt.Sprintf("The article link: %s\nThe time remaining in seconds: %v", req.ArticleLink, req.TimeLimitSeconds)

	cfg.generateAndStreamResponse(w, r, newSession, initialPrompt)
}

type SendMessageResponse struct {
	LLMResponse string `json:"llmResponse"`
	Message     string `json:"message"`
	Error       string `json:"error,omitempty"`
}

func (cfg *config) sendMessageHandler(w http.ResponseWriter, r *http.Request) {

}

func resWithError(w http.ResponseWriter, code int, msg string) {
	if err := resWithJson(w, code, struct{ Error string }{Error: msg}); err != nil {
		http.Error(w, err.Error(), code)
	}
}

func resWithJson(w http.ResponseWriter, code int, payload any) error {
	dat, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.WriteHeader(code)
	w.Write(dat)
	return nil
}
