package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/infra/tenant"
)

type Turn struct {
	SessionID  int64
	Role       string
	Body       string
	CiteKey    string
	CiteValues []string
	ToolID     string
	Outcome    string
}

const sessionLookupQuery = `SELECT id FROM chat_sessions WHERE id = ? AND company_id = ? AND user_id = ?`

const lastCitesQuery = `
SELECT t.cite_key, t.cite_values
FROM chat_turns t
JOIN chat_sessions s ON s.id = t.session_id
WHERE t.session_id = ? AND s.company_id = ? AND s.user_id = ?
  AND t.role = 'assistant' AND t.cite_values IS NOT NULL
ORDER BY t.id DESC
LIMIT 1`

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func validRole(role string) bool {
	return role == "user" || role == "assistant"
}

func (r *Repository) EnsureSession(ctx context.Context, companyID, userID, sessionID int64) (int64, error) {
	if companyID <= 0 || userID <= 0 {
		return 0, errors.New("chatsession: company and user scope are required")
	}
	if sessionID > 0 {
		var found int64
		err := r.db.QueryRowContext(ctx, sessionLookupQuery, sessionID, companyID, userID).Scan(&found)
		if err == nil {
			return found, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("chatsession: load: %w", err)
		}
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO chat_sessions (company_id, user_id, last_turn_at) VALUES (?, ?, NOW())`,
		companyID, userID)
	if err != nil {
		return 0, fmt.Errorf("chatsession: create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("chatsession: create id: %w", err)
	}
	return id, nil
}

func (r *Repository) LastCites(ctx context.Context, companyID, userID, sessionID int64) (string, []string, error) {
	if sessionID <= 0 || companyID <= 0 || userID <= 0 {
		return "", nil, nil
	}
	var citeKey sql.NullString
	var raw sql.NullString
	err := r.db.QueryRowContext(ctx, lastCitesQuery, sessionID, companyID, userID).Scan(&citeKey, &raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("chatsession: last cites: %w", err)
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return citeKey.String, nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw.String), &values); err != nil {
		return citeKey.String, nil, fmt.Errorf("chatsession: parse cite values: %w", err)
	}
	return citeKey.String, values, nil
}

func (r *Repository) AppendTurn(ctx context.Context, turn Turn) error {
	if turn.SessionID <= 0 {
		return errors.New("chatsession: session id is required")
	}
	if !validRole(turn.Role) {
		return fmt.Errorf("chatsession: role %q must be user or assistant", turn.Role)
	}
	var encoded any
	if len(turn.CiteValues) > 0 {
		raw, err := json.Marshal(turn.CiteValues)
		if err != nil {
			return fmt.Errorf("chatsession: encode cite values: %w", err)
		}
		encoded = string(raw)
	}
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO chat_turns (session_id, role, body, cite_key, cite_values, tool_id, outcome)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		turn.SessionID, turn.Role, turn.Body,
		nullableString(turn.CiteKey), encoded,
		nullableString(turn.ToolID), nullableString(turn.Outcome)); err != nil {
		return fmt.Errorf("chatsession: append turn: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE chat_sessions SET last_turn_at = NOW() WHERE id = ?`, turn.SessionID); err != nil {
		return fmt.Errorf("chatsession: touch session: %w", err)
	}
	return nil
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
