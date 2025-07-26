package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

type config struct {
	port   string
	apiKey string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("couldn't get enivornment variables: ", err)
	}

	apiKey := os.Getenv("API_KEY")
	port := os.Getenv("PORT")

	cfg := config{
		port:   port,
		apiKey: apiKey,
	}
	mux := http.NewServeMux()
	var s http.Server
	s.Addr = cfg.port
	s.Handler = mux

	mux.HandleFunc("GET /health", health)

	log.Printf("server started on port: %v ...", s.Addr)
	log.Fatal(s.ListenAndServe())
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("OK"))
}
