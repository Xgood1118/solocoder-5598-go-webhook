package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

func generateHMAC(payload []byte, timestamp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	tsStr := strconv.FormatInt(timestamp, 10)
	mac.Write([]byte(tsStr))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func main() {
	payload := []byte(`{"user_id":123,"name":"test user"}`)
	secret := "my-secret-key-123"
	timestamp := time.Now().Unix()
	signature := generateHMAC(payload, timestamp, secret)

	fmt.Printf("Timestamp: %d\n", timestamp)
	fmt.Printf("Signature: %s\n", signature)

	req, err := http.NewRequest("POST", "http://localhost:8084/webhook/receive", bytes.NewReader(payload))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", "evt-test-go-002")
	req.Header.Set("X-Event-Type", "user.created")
	req.Header.Set("X-Key-ID", "key-1")
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Signature", signature)

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("\nResponse Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Body: %s\n", string(body))
	fmt.Printf("Latency: %v\n", latency)
}
