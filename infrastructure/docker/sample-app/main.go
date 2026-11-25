package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	version := os.Getenv("APP_VERSION")
	if version == "" {
		version = "v1.0.0"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "healthy",
			"service":   "infraos-sample-workload",
			"version":   version,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service": "infraos-sample-workload",
			"version": version,
		})
	})

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("sample workload listening", "port", port, "version", version)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		logger.Error("sample workload failed", "error", err)
		os.Exit(1)
	}
}
