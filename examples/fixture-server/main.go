// Command fixture-server is a tiny standalone HTTP API for exercising
// apilens run/test end to end (plan.md v1 "examples/httpbin-or-fixture/").
// It is not part of the apilens module's public API — just a fixture.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"sync"
)

type user struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

var (
	mu    sync.Mutex
	users = []user{
		{ID: 1, Name: "Ada Lovelace", Email: "ada@example.com"},
		{ID: 2, Name: "Alan Turing", Email: "alan@example.com"},
	}
	nextID = 3
)

func main() {
	// Default 5050, not 5000: port 5000 collides with macOS AirPlay
	// Receiver (ControlCenter) on many systems.
	addr := flag.String("addr", ":5050", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/users", handleUsers)
	mux.HandleFunc("/api/users/", handleUserByID)

	log.Printf("fixture-server listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"data": users})
	case http.MethodPost:
		var body struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		mu.Lock()
		u := user{ID: nextID, Name: body.Name, Email: body.Email}
		nextID++
		users = append(users, u)
		mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"data": u})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleUserByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/users/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, u := range users {
		if u.ID == id {
			writeJSON(w, http.StatusOK, map[string]any{"data": u})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
