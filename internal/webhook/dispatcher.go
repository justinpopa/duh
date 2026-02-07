package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/safenet"
)

type Event struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

type Dispatcher struct {
	db   *sql.DB
	ch   chan Event
	done chan struct{}
}

func NewDispatcher(database *sql.DB) *Dispatcher {
	d := &Dispatcher{
		db:   database,
		ch:   make(chan Event, 100),
		done: make(chan struct{}),
	}
	go d.worker()
	return d
}

func (d *Dispatcher) Fire(event Event) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	select {
	case d.ch <- event:
	default:
		log.Printf("webhook: event channel full, dropping %s event", event.Type)
	}
}

func (d *Dispatcher) Close() {
	close(d.ch)
	<-d.done
}

func (d *Dispatcher) worker() {
	defer close(d.done)
	client := safenet.NewClient(10 * time.Second)

	for event := range d.ch {
		webhooks, err := db.ListEnabledWebhooks(d.db)
		if err != nil {
			log.Printf("webhook: list enabled: %v", err)
			continue
		}

		body, err := json.Marshal(event)
		if err != nil {
			log.Printf("webhook: marshal event: %v", err)
			continue
		}

		for _, wh := range webhooks {
			if !matchEvent(wh.Events, event.Type) {
				continue
			}
			d.deliver(client, wh, body)
		}
	}
}

func (d *Dispatcher) deliver(client *http.Client, wh db.Webhook, body []byte) {
	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: create request for %s: %v", wh.URL, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", sig)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook: POST %s: %v", wh.URL, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("webhook: POST %s: status %d", wh.URL, resp.StatusCode)
	}
}

func matchEvent(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	for _, p := range strings.Split(pattern, ",") {
		if strings.TrimSpace(p) == eventType {
			return true
		}
	}
	return false
}

// DeliverSingle sends a single event to a specific webhook synchronously.
// Used for the test endpoint.
func DeliverSingle(wh db.Webhook, event Event) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", sig)
	}

	client := safenet.NewClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
