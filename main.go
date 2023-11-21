package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

var (
	connections = struct {
		sync.Mutex
		m map[string]chan string
	}{m: make(map[string]chan string)}
)

func prepareHeaderForSSE(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Access-Control-Allow-Origin", "*")
}

func writeNotification(w http.ResponseWriter, connection chan string) (int, error) {
	message, ok := <-connection
	if !ok {
		return 0, fmt.Errorf("channel closed")
	}

	return fmt.Fprintf(w, "data: %s\n\n", message)
}

func addConnection(username string, connection chan string) {
	connections.Lock()
	defer connections.Unlock()
	connections.m[username] = connection
}

func sendMessage(to string, message string) {
	connections.Lock()
	defer connections.Unlock()
	if connection, ok := connections.m[to]; ok {
		connection <- message
	}
}

func renderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	tmplPath := "templates/" + tmpl
	t, err := template.ParseFiles(tmplPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = t.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	r := chi.NewRouter()

	r.Get("/users/{username}/notifications", func(w http.ResponseWriter, r *http.Request) {
		username := chi.URLParam(r, "username")
		renderTemplate(w, "index.html", map[string]interface{}{
			"Username": username,
		})
	})

	r.Get("/users/{username}/notifications/listen", func(w http.ResponseWriter, r *http.Request) {
		username := chi.URLParam(r, "username")

		listenNotification := func(w http.ResponseWriter, r *http.Request) {
			prepareHeaderForSSE(w)

			connection := make(chan string)
			addConnection(username, connection)

			for {
				_, err := writeNotification(w, connection) // blocking until there is a message
				if err != nil {
					break
				}

				flusher, ok := w.(http.Flusher)
				if !ok {
					break
				}
				flusher.Flush()
			}

			connections.Lock()
			defer connections.Unlock()
			delete(connections.m, username)
		}

		listenNotification(w, r)
	})

	r.Post("/users/{username}/notifications", func(w http.ResponseWriter, r *http.Request) {
		username := chi.URLParam(r, "username")
		message := r.URL.Query().Get("message")

		sendMessage(username, message)
	})

	log.Println("Listening on localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
