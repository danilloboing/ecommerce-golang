package testutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignWebhook returns hex(HMAC-SHA256(secret, body)). It mirrors the signing
// algorithm used by the payment mock provider (infrastructure.Sign) so E2E
// tests can forge a valid X-Webhook-Signature header without importing the
// payment package.
func SignWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
