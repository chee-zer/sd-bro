package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	var s http.Server
	s.Addr = ":8080"
	s.Handler = mux

	mux.HandleFunc("GET /health", health)

	log.Fatal(s.ListenAndServe())
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("OK"))
}
