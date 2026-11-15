package forwarder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"token-ray/internal/model"
)

// Forwarder listens to a usage channel and forwards events to a remote HTTP endpoint.
type Forwarder struct {
	targetURL string
	token     string
	deviceID  string
	client    *http.Client
}

func New(targetURL, token, deviceID string) *Forwarder {
	return &Forwarder{
		targetURL: targetURL,
		token:     token,
		deviceID:  deviceID,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Start consumes usage events from the channel and sends them concurrently.
// It returns when the channel is closed and all events have been sent.
func (f *Forwarder) Start(usageChan <-chan model.TokenUsage) {
	var wg sync.WaitGroup
	for usage := range usageChan {
		wg.Add(1)
		go func(u model.TokenUsage) {
			defer wg.Done()
			err := f.send(u)
			if err != nil {
				log.Printf("Forwarder failed to send usage: %v", err)
			} else {
				log.Printf("Forwarded %d tokens for %s", u.InputTokens+u.OutputTokens, u.Model)
			}
		}(usage)
	}
	wg.Wait()
}

func (f *Forwarder) send(usage model.TokenUsage) error {
	payload := model.TrackPayload{
		DeviceID: f.deviceID,
		Usage:    usage,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Simple retry loop
	var lastErr error
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("POST", f.targetURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if f.token != "" {
			req.Header.Set("Authorization", "Bearer "+f.token)
		}

		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return nil
		}
		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		resp.Body.Close()
		time.Sleep(1 * time.Second)
	}
	return lastErr
}
