package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"webhook-service/internal/model"
	"webhook-service/internal/store"
)

const (
	TimestampToleranceSec = 300
)

type SignatureVerifier struct {
	store *store.Store
}

func NewSignatureVerifier(store *store.Store) *SignatureVerifier {
	return &SignatureVerifier{store: store}
}

type VerifyResult struct {
	Valid   bool
	KeyID   string
	Error   string
}

func (sv *SignatureVerifier) VerifyEndpoint(endpointID string, payload []byte, timestamp int64, signature string, keyID string) *VerifyResult {
	ep, err := sv.store.GetEndpoint(endpointID)
	if err != nil {
		return &VerifyResult{Valid: false, Error: "endpoint not found"}
	}
	return sv.VerifyWithKeys(ep.APIKeys, payload, timestamp, signature, keyID)
}

func (sv *SignatureVerifier) VerifyWithKeys(keys []model.APIKeyPair, payload []byte, timestamp int64, signature string, keyID string) *VerifyResult {
	if len(keys) == 0 {
		return &VerifyResult{Valid: false, Error: "no keys configured"}
	}

	now := time.Now().Unix()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > TimestampToleranceSec {
		return &VerifyResult{Valid: false, Error: "timestamp out of tolerance"}
	}

	var targetKey *model.APIKeyPair
	for i := range keys {
		if keys[i].KeyID == keyID {
			targetKey = &keys[i]
			break
		}
	}

	if targetKey == nil {
		return &VerifyResult{Valid: false, Error: "key_id not found"}
	}

	expectedSig := GenerateHMAC(payload, timestamp, targetKey.Secret)

	if !hmac.Equal([]byte(expectedSig), []byte(signature)) {
		return &VerifyResult{Valid: false, KeyID: keyID, Error: "signature mismatch"}
	}

	return &VerifyResult{Valid: true, KeyID: keyID}
}

func GenerateHMAC(payload []byte, timestamp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	tsStr := strconv.FormatInt(timestamp, 10)
	mac.Write([]byte(tsStr))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func ParseTimestamp(tsStr string) (int64, error) {
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp: %w", err)
	}
	return ts, nil
}
