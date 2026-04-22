package streaming

import (
	"encoding/json"
	"net/http"
	"time"
)

type EventType string

const (
	StartEvent EventType = "start"
	TextEvent  EventType = "text"
	DoneEvent  EventType = "done"
	ErrorEvent EventType = "error"
)

type StreamEvent struct {
	Type       EventType `json:"type"`
	Content    string    `json:"content"`
	TokenUsage int       `json:"token_usage,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

type Streamer struct {
	w http.ResponseWriter
}

func (s *Streamer) Send(event StreamEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.w.Write([]byte("data: " + string(data) + "\n\n"))
	return err
}

func HandleStreaming(w http.ResponseWriter, r *http.Request, handler func(Streamer) error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	streamer := Streamer{w: w}
	if err := handler(streamer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	flusher.Flush()
}
