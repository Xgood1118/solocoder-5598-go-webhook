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
	payload := []byte(`{"action":"retry-test"}`)
	secret := "s1"
	timestamp := time.Now().Unix()
	signature := generateHMAC(payload, timestamp, secret)

	req, err := http.NewRequest("POST", "http://localhost:8084/webhook/receive", bytes.NewReader(payload))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", "evt-retry-offbyone-001")
	req.Header.Set("X-Event-Type", "retry.test")
	req.Header.Set("X-Key-ID", "k1")
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Signature", signature)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\nBody: %s\n", resp.StatusCode, string(body))
}
