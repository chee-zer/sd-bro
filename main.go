package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/vertexai/genai"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

// configss
type config struct {
	port         string
	clientConfig ClientConfig
}

type ClientConfig struct {
	Project  string
	Location string
}

type ChatSession struct {
	ID               string
	ChatHistory      []*genai.Content
	ArticleURL       string
	StartTime        time.Time
	TimeLimitSeconds int
	IsActive         bool
	LastActivityTime time.Time
}

var (
	chatSessions  = make(map[string]*ChatSession)
	sessionsMutex sync.RWMutex
)

func main() {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		log.Fatal("PROJECT_ID environment variable not set")
	}
	location := os.Getenv("LOCATION")
	if location == "" {
		log.Fatal("LOCATION environment variable not set")
	}

	clientConfig := ClientConfig{
		Project:  projectID,
		Location: location,
	}
	cfg := config{
		port:         port,
		clientConfig: clientConfig,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("POST /start", cfg.startChatHandler)
	mux.HandleFunc("POST /chat/{sessionId}", cfg.chatHandler)
	mux.HandleFunc("POST /stt", cfg.sttHandler)
	mux.HandleFunc("POST /tts", cfg.ttsHandler)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:5173", "http://127.0.0.1:3000"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})

	handler := c.Handler(mux)

	s := http.Server{
		Addr:    ":" + cfg.port,
		Handler: handler,
	}

	log.Printf("server listening on port: %v ...", s.Addr)
	log.Fatal(s.ListenAndServe())
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("OK"))
}

// types
type StartChatRequest struct {
	ArticleLink      string `json:"articleLink"`
	TimeLimitSeconds int    `json:"timeLimitSeconds"`
}

type StartChatResponse struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

type ChatRequest struct {
	UserMessage string `json:"userMessage"`
}

type ChatResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

type TtsRequest struct {
	Text string `json:"text"`
}

type SttResponse struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

//handlers

func (cfg *config) startChatHandler(w http.ResponseWriter, r *http.Request) {
	var req StartChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.ArticleLink == "" || !isURL(req.ArticleLink) {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid 'articleLink' is required"})
		return
	}

	if req.TimeLimitSeconds <= 0 {
		req.TimeLimitSeconds = 300 // Default to 5 minutes
	}

	sessionID := uuid.New().String()
	newSession := &ChatSession{
		ID:               sessionID,
		ArticleURL:       req.ArticleLink,
		StartTime:        time.Now(),
		TimeLimitSeconds: req.TimeLimitSeconds,
		IsActive:         true,
		LastActivityTime: time.Now(),
		ChatHistory:      buildInitialPrompt(req.ArticleLink, req.TimeLimitSeconds),
	}

	llmResponse, err := cfg.generateResponse(r.Context(), newSession)
	if err != nil {
		respondWithJSON(w, http.StatusInternalServerError, StartChatResponse{Error: err.Error()})
		return
	}

	newSession.ChatHistory = append(newSession.ChatHistory, &genai.Content{
		Parts: []genai.Part{genai.Text(llmResponse)},
		Role:  "model",
	})

	sessionsMutex.Lock()
	chatSessions[sessionID] = newSession
	sessionsMutex.Unlock()

	log.Println("New session started and initial response generated: ", sessionID)

	respondWithJSON(w, http.StatusCreated, StartChatResponse{
		SessionID: sessionID,
		Message:   llmResponse,
	})
}

func (cfg *config) chatHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing session ID in URL path"})
		return
	}

	sessionsMutex.RLock()
	session, ok := chatSessions[sessionID]
	sessionsMutex.RUnlock()

	if !ok || !session.IsActive {
		respondWithJSON(w, http.StatusNotFound, map[string]string{"error": "Session not found or has expired"})
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.UserMessage == "" {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "User message cannot be empty"})
		return
	}

	session.ChatHistory = append(session.ChatHistory, &genai.Content{
		Parts: []genai.Part{genai.Text(req.UserMessage), genai.Text("time remaining : ")},
		Role:  "user",
	})
	session.LastActivityTime = time.Now()

	llmResponse, err := cfg.generateResponse(r.Context(), session)
	if err != nil {
		respondWithJSON(w, http.StatusInternalServerError, ChatResponse{Error: err.Error()})
		return
	}

	session.ChatHistory = append(session.ChatHistory, &genai.Content{
		Parts: []genai.Part{genai.Text(llmResponse)},
		Role:  "model",
	})

	respondWithJSON(w, http.StatusOK, ChatResponse{Message: llmResponse})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal server error"}`))
		return
	}
	w.WriteHeader(code)
	w.Write(response)
}

func (cfg *config) sttHandler(w http.ResponseWriter, r *http.Request) {
	//10 mb
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondWithJSON(w, http.StatusBadRequest, SttResponse{Error: "Could not parse form"})
		return
	}

	file, _, err := r.FormFile("audio")
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, SttResponse{Error: "Form file 'audio' is required"})
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		respondWithJSON(w, http.StatusInternalServerError, SttResponse{Error: "Could not read audio file"})
		return
	}

	transcript, err := cfg.convertSpeechToText(r.Context(), audioData)
	if err != nil {
		respondWithJSON(w, http.StatusInternalServerError, SttResponse{Error: "Failed to process audio"})
		return
	}

	respondWithJSON(w, http.StatusOK, SttResponse{Text: transcript})
}

func (cfg *config) ttsHandler(w http.ResponseWriter, r *http.Request) {
	var req TtsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.Text == "" {
		respondWithJSON(w, http.StatusBadRequest, map[string]string{"error": "Text cannot be empty"})
		return
	}

	audioData, err := cfg.convertTextToSpeech(r.Context(), req.Text)
	if err != nil {
		respondWithJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate audio"})
		return
	}
	//for streaming
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(audioData)))
	w.Header().Set("Accept-Ranges", "bytes")

	w.WriteHeader(http.StatusOK)

	w.Write(audioData)
}

func isURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func (cs *ChatSession) IsTimeExceeded() bool {
	return time.Since(cs.StartTime) > time.Duration(cs.TimeLimitSeconds)*time.Second
}

func (cs *ChatSession) TimeRemaining() time.Duration {
	elapsed := time.Since(cs.StartTime)
	remaining := time.Duration(cs.TimeLimitSeconds)*time.Second - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}
