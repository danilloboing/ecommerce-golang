package sessionauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisOptions configures RedisManager.
type RedisOptions struct {
	Client        *redis.Client
	TTLDefault    time.Duration
	TTLRememberMe time.Duration
	RefreshAfter  time.Duration
	Now           func() time.Time
}

// RedisManager stores sessions as hashes in Redis with a per-user index set.
type RedisManager struct {
	client        *redis.Client
	ttlDefault    time.Duration
	ttlRememberMe time.Duration
	refreshAfter  time.Duration
	now           func() time.Time
}

var _ Manager = (*RedisManager)(nil)

// NewRedisManager builds a RedisManager.
func NewRedisManager(opts RedisOptions) *RedisManager {
	if opts.TTLDefault == 0 {
		opts.TTLDefault = 14 * 24 * time.Hour
	}
	if opts.TTLRememberMe == 0 {
		opts.TTLRememberMe = 30 * 24 * time.Hour
	}
	if opts.RefreshAfter == 0 {
		opts.RefreshAfter = 24 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &RedisManager{
		client:        opts.Client,
		ttlDefault:    opts.TTLDefault,
		ttlRememberMe: opts.TTLRememberMe,
		refreshAfter:  opts.RefreshAfter,
		now:           opts.Now,
	}
}

func sessionKey(id string) string       { return "session:" + id }
func userIndexKey(uid uuid.UUID) string { return "session:user:" + uid.String() }

// Create stores a new session and adds it to the per-user index.
func (m *RedisManager) Create(ctx context.Context, p CreateParams) (Session, error) {
	id, err := randomHex(32)
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: random session id: %w", err)
	}
	csrf, err := randomHex(32)
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: random csrf: %w", err)
	}

	ttl := m.ttlDefault
	if p.RememberMe {
		ttl = m.ttlRememberMe
	}
	now := m.now().UTC()
	expiresAt := now.Add(ttl)

	sess := Session{
		ID:             id,
		UserID:         p.UserID,
		CSRFToken:      csrf,
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      expiresAt,
		RememberMe:     p.RememberMe,
		UserAgent:      p.UserAgent,
		IP:             p.IP,
	}

	pipe := m.client.TxPipeline()
	pipe.HSet(ctx, sessionKey(id), serializeSession(sess))
	pipe.ExpireAt(ctx, sessionKey(id), expiresAt)
	pipe.SAdd(ctx, userIndexKey(p.UserID), id)
	pipe.ExpireAt(ctx, userIndexKey(p.UserID), expiresAt)
	if _, err := pipe.Exec(ctx); err != nil {
		return Session{}, fmt.Errorf("sessionauth: create pipeline: %w", err)
	}
	return sess, nil
}

// Get fetches a session by id. Returns ErrNotFound if missing.
func (m *RedisManager) Get(ctx context.Context, sessionID string) (Session, error) {
	res, err := m.client.HGetAll(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: hgetall: %w", err)
	}
	if len(res) == 0 {
		return Session{}, ErrNotFound
	}
	return deserializeSession(sessionID, res)
}

// Refresh advances LastActivityAt and renews TTL for both keys.
func (m *RedisManager) Refresh(ctx context.Context, sessionID string) error {
	sess, err := m.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	now := m.now().UTC()
	if now.Sub(sess.LastActivityAt) < m.refreshAfter {
		// No-op; not stale enough to warrant a Redis write.
		return nil
	}
	sess.LastActivityAt = now

	ttl := m.ttlDefault
	if sess.RememberMe {
		ttl = m.ttlRememberMe
	}
	expiresAt := now.Add(ttl)
	sess.ExpiresAt = expiresAt

	pipe := m.client.TxPipeline()
	pipe.HSet(ctx, sessionKey(sessionID), serializeSession(sess))
	pipe.ExpireAt(ctx, sessionKey(sessionID), expiresAt)
	pipe.ExpireAt(ctx, userIndexKey(sess.UserID), expiresAt)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sessionauth: refresh pipeline: %w", err)
	}
	return nil
}

// Delete removes a single session and its index entry.
func (m *RedisManager) Delete(ctx context.Context, sessionID string) error {
	sess, err := m.Get(ctx, sessionID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	pipe := m.client.TxPipeline()
	pipe.Del(ctx, sessionKey(sessionID))
	pipe.SRem(ctx, userIndexKey(sess.UserID), sessionID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sessionauth: delete pipeline: %w", err)
	}
	return nil
}

// DeleteAllForUser removes every session for the given user.
func (m *RedisManager) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	ids, err := m.client.SMembers(ctx, userIndexKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("sessionauth: smembers: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	keys := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		keys = append(keys, sessionKey(id))
	}
	keys = append(keys, userIndexKey(userID))

	if _, err := m.client.Del(ctx, keys...).Result(); err != nil {
		return fmt.Errorf("sessionauth: del all: %w", err)
	}
	return nil
}

// DeleteAllForUserExcept keeps the session keepID and deletes the rest.
func (m *RedisManager) DeleteAllForUserExcept(ctx context.Context, userID uuid.UUID, keepID string) error {
	ids, err := m.client.SMembers(ctx, userIndexKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("sessionauth: smembers: %w", err)
	}

	pipe := m.client.TxPipeline()
	for _, id := range ids {
		if id == keepID {
			continue
		}
		pipe.Del(ctx, sessionKey(id))
		pipe.SRem(ctx, userIndexKey(userID), id)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sessionauth: prune pipeline: %w", err)
	}
	return nil
}

// --- helpers ---

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func serializeSession(s Session) map[string]string {
	return map[string]string{
		"user_id":          s.UserID.String(),
		"csrf_token":       s.CSRFToken,
		"created_at":       strconv.FormatInt(s.CreatedAt.UnixNano(), 10),
		"last_activity_at": strconv.FormatInt(s.LastActivityAt.UnixNano(), 10),
		"expires_at":       strconv.FormatInt(s.ExpiresAt.UnixNano(), 10),
		"remember_me":      strconv.FormatBool(s.RememberMe),
		"user_agent":       s.UserAgent,
		"ip":               s.IP,
	}
}

func deserializeSession(id string, data map[string]string) (Session, error) {
	uid, err := uuid.Parse(data["user_id"])
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: parse user_id: %w", err)
	}
	created, err := parseUnixNano(data["created_at"])
	if err != nil {
		return Session{}, err
	}
	last, err := parseUnixNano(data["last_activity_at"])
	if err != nil {
		return Session{}, err
	}
	expires, err := parseUnixNano(data["expires_at"])
	if err != nil {
		return Session{}, err
	}
	remember, _ := strconv.ParseBool(data["remember_me"])

	return Session{
		ID:             id,
		UserID:         uid,
		CSRFToken:      data["csrf_token"],
		CreatedAt:      created,
		LastActivityAt: last,
		ExpiresAt:      expires,
		RememberMe:     remember,
		UserAgent:      data["user_agent"],
		IP:             data["ip"],
	}, nil
}

func parseUnixNano(s string) (time.Time, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("sessionauth: parse time %q: %w", s, err)
	}
	return time.Unix(0, n).UTC(), nil
}
