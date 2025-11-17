package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
)

type movieEntry struct {
	Title       string           `json:"title"`
	Distributor *string          `json:"distributor"`
	ReleaseDate *string          `json:"releaseDate"`
	Budget      *int64           `json:"budget"`
	Revenue     *json.RawMessage `json:"revenue"`
	MpaRating   *string          `json:"mpaRating"`
}

func main() {
	var (
		port    = flag.String("port", "9099", "port to listen on")
		data    = flag.String("data", "mock-boxoffice.json", "path to mock data file")
		logJSON = flag.Bool("log", false, "enable request logging")
	)
	flag.Parse()

	file, err := os.ReadFile(*data)
	if err != nil {
		log.Fatalf("read mock data: %v", err)
	}

	var payload map[string]movieEntry
	if err := json.Unmarshal(file, &payload); err != nil {
		log.Fatalf("parse mock data: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/boxoffice", func(w http.ResponseWriter, r *http.Request) {
		title := r.URL.Query().Get("title")
		entry, ok := payload[title]
		if !ok {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entry); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	addr := ":" + *port
	log.Printf("mock boxoffice listening on %s", addr)
	if *logJSON {
		log.Printf("loaded %d mock entries", len(payload))
	}
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
