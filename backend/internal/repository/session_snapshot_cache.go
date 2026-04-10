package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const sessionSnapshotKeyPrefix = "control:session:snapshot:"

type sessionSnapshotCache struct {
	rdb *redis.Client
}

func NewSessionSnapshotCache(rdb *redis.Client) service.SessionSnapshotCache {
	return &sessionSnapshotCache{rdb: rdb}
}

func (c *sessionSnapshotCache) GetSessionSnapshot(ctx context.Context, sessionID string) (*service.SessionSnapshot, error) {
	if c == nil || c.rdb == nil || sessionID == "" {
		return nil, nil
	}

	data, err := c.rdb.Get(ctx, sessionSnapshotKeyPrefix+sessionID).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get session snapshot: %w", err)
	}

	var snapshot service.SessionSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal session snapshot: %w", err)
	}
	return &snapshot, nil
}

func (c *sessionSnapshotCache) SetSessionSnapshot(ctx context.Context, snapshot *service.SessionSnapshot, ttl time.Duration) error {
	if c == nil || c.rdb == nil || snapshot == nil || snapshot.SessionID == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal session snapshot: %w", err)
	}

	if err := c.rdb.Set(ctx, sessionSnapshotKeyPrefix+snapshot.SessionID, data, ttl).Err(); err != nil {
		return fmt.Errorf("set session snapshot: %w", err)
	}
	return nil
}

func (c *sessionSnapshotCache) DeleteSessionSnapshot(ctx context.Context, sessionID string) error {
	if c == nil || c.rdb == nil || sessionID == "" {
		return nil
	}
	if err := c.rdb.Del(ctx, sessionSnapshotKeyPrefix+sessionID).Err(); err != nil {
		return fmt.Errorf("delete session snapshot: %w", err)
	}
	return nil
}
