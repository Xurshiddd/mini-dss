package store

import (
	"context"
	"time"
)

func (s *Store) RevokeToken(ctx context.Context, hash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO revoked_tokens (token_hash, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (token_hash) DO UPDATE SET expires_at = EXCLUDED.expires_at`,
		hash, expiresAt)
	return err
}

func (s *Store) ActiveRevokedTokens(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT token_hash, expires_at FROM revoked_tokens WHERE expires_at > now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var hash string
		var expires time.Time
		if err := rows.Scan(&hash, &expires); err != nil {
			return nil, err
		}
		out[hash] = expires
	}
	return out, rows.Err()
}

func (s *Store) CleanupRevokedTokens(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM revoked_tokens WHERE expires_at <= now()`)
	return err
}
