package main

import (
	"encoding/json"
	"time"
)

// Event is a single log entry emitted as a JSON line to stdout.
type Event struct {
	TS     time.Time       `json:"ts"`
	Source string          `json:"source"`
	Zone   string          `json:"zone,omitempty"`
	Data   json.RawMessage `json:"data"`
}
