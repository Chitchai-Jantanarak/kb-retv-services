package skeleton

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/shared/perms"
)

const pendingTTL = 10 * time.Minute

type PendingRef struct {
	ID      string
	Summary string
}

type pendingEntry struct {
	companyID int64
	userID    int64
	toolID    string
	params    map[string]string
	expires   time.Time
}

type PendingStore struct {
	mu      sync.Mutex
	entries map[string]pendingEntry
	now     func() time.Time
}

func NewPendingStore() *PendingStore {
	return &PendingStore{entries: make(map[string]pendingEntry), now: time.Now}
}

func (s *PendingStore) Put(actor Actor, toolID string, params map[string]string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	id := hex.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.entries {
		if s.now().After(entry.expires) {
			delete(s.entries, key)
		}
	}
	s.entries[id] = pendingEntry{
		companyID: actor.CompanyID,
		userID:    actor.UserID,
		toolID:    toolID,
		params:    params,
		expires:   s.now().Add(pendingTTL),
	}
	return id
}

func (s *PendingStore) Take(actor Actor, id string) (string, map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return "", nil, false
	}
	delete(s.entries, id)
	if entry.companyID != actor.CompanyID || entry.userID != actor.UserID {
		return "", nil, false
	}
	if s.now().After(entry.expires) {
		return "", nil, false
	}
	return entry.toolID, entry.params, true
}

func (o *Orchestrator) Confirm(ctx context.Context, actor Actor, id string) (Response, error) {
	if o.pending == nil {
		return Response{}, fmt.Errorf("skeleton: confirm: no pending store configured")
	}
	toolID, params, ok := o.pending.Take(actor, id)
	if !ok {
		return Response{}, fmt.Errorf("skeleton: confirm: action not found, already executed, or expired")
	}

	tool, ok := o.catalog[toolID]
	if !ok {
		return Response{}, fmt.Errorf("skeleton: confirm: unknown tool %q", toolID)
	}
	if !perms.Can(actor.Perms, tool.RBAC.RequiresPermission) {
		return Response{Kind: tool.Kind}, fmt.Errorf("skeleton: confirm: permission denied for %q", toolID)
	}

	handler, ok := o.handlers[tool.Handler]
	if !ok {
		return Response{Kind: tool.Kind}, fmt.Errorf("skeleton: confirm: no handler %q", tool.Handler)
	}

	if err := o.auditor.Log(ctx, tool.Audit, actor, tool.ID); err != nil {
		return Response{Kind: tool.Kind}, fmt.Errorf("skeleton: confirm: audit: %w", err)
	}

	rows, err := handler.Run(ctx, Query{Tool: tool, Params: params, Coverage: actor.Coverage, Actor: actor})
	if err != nil {
		return Response{Kind: tool.Kind}, fmt.Errorf("skeleton: confirm: handler %q: %w", tool.Handler, err)
	}

	composed := compose(tool, rows, params)
	composed.Called = true
	composed.Params = cloneParams(params)
	return composed, nil
}

func proposalSummary(tool tools.Tool, params map[string]string) string {
	return tidyTemplate(fillTemplate(tool.Compose.Headline, Row(params)))
}
