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

func send(eventType, keyID, secret string, eventID string) {
	payload := []byte(`{"x":1}`)
	timestamp := time.Now().Unix()
	signature := generateHMAC(payload, timestamp, secret)

	req, _ := http.NewRequest("POST", "http://localhost:8084/webhook/receive", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", eventID)
	req.Header.Set("X-Event-Type", eventType)
	req.Header.Set("X-Key-ID", keyID)
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Signature", signature)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[%s] key=%s evt=%s => HTTP %d | %s\n", eventID, keyID, eventType, resp.StatusCode, string(body))
}

func main() {
	send("bar.event", "shared-key", "aaa", "cross-test-1")
	send("foo.event", "shared-key", "aaa", "cross-test-2")
}
