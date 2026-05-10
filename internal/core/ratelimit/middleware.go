package ratelimit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/redis/go-redis/v9"
)

// Source identifies how a rule keys its bucket.
type Source int

// Supported rule sources.
const (
	SourceUnknown Source = iota
	ByIP
	ByEmailField
	ByUserID
)

// Rule defines a single rate-limit constraint applied per request.
type Rule struct {
	Key    string
	Source Source
	Field  string // for ByEmailField — JSON path (e.g. "email")
	Limit  int
	Window time.Duration
}

// Options configures the middleware.
type Options struct {
	Client         *redis.Client
	Rules          []Rule
	TrustedProxies []netip.Prefix
}

// incrScript is an atomic INCR + EXPIRE-on-first-hit for fixed-window counters.
const incrScript = `local v = redis.call('INCR', KEYS[1])
if v == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return v`

// Middleware enforces all configured rules on every request.
// Each rule's key is computed from request data; the first rule to exceed
// its limit short-circuits with 429 and a Retry-After header.
func Middleware(opts Options) func(http.Handler) http.Handler {
	script := redis.NewScript(incrScript)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Buffer body once if any rule needs JSON field access.
			var bodyBuf []byte
			needBody := false
			for _, rule := range opts.Rules {
				if rule.Source == ByEmailField {
					needBody = true
					break
				}
			}
			if needBody && r.Body != nil {
				buf, err := io.ReadAll(r.Body)
				if err == nil {
					bodyBuf = buf
					r.Body = io.NopCloser(bytes.NewReader(buf))
				}
			}

			now := time.Now().Unix()
			for _, rule := range opts.Rules {
				windowSecs := int64(rule.Window.Seconds())
				if windowSecs <= 0 {
					continue
				}
				bucket := now / windowSecs
				keyPart, ok := keyForRule(r, rule, opts.TrustedProxies, bodyBuf)
				if !ok {
					continue
				}
				key := fmt.Sprintf("ratelimit:%s:%s:%d", rule.Key, keyPart, bucket)

				count, err := script.Run(r.Context(), opts.Client, []string{key},
					int(windowSecs)).Int()
				if err != nil {
					responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "rate limit failure", err)
					return
				}
				if count > rule.Limit {
					retry := windowSecs - (now % windowSecs)
					w.Header().Set("Retry-After", strconv.FormatInt(retry, 10))
					responsex.Error(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func keyForRule(r *http.Request, rule Rule, trusted []netip.Prefix, body []byte) (string, bool) {
	switch rule.Source {
	case ByIP:
		return "ip:" + RealIP(r, trusted), true
	case ByUserID:
		sess, ok := sessionauth.SessionFromContext(r.Context())
		if !ok {
			return "", false
		}
		return "user:" + sess.UserID.String(), true
	case ByEmailField:
		if len(body) == 0 || rule.Field == "" {
			return "", false
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", false
		}
		v, ok := payload[rule.Field].(string)
		if !ok || v == "" {
			return "", false
		}
		sum := sha256.Sum256([]byte(v))
		return "email:" + hex.EncodeToString(sum[:]), true
	}
	return "", false
}
