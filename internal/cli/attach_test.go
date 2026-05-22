package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestResolveSessionByIDAndLabel(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now()
	oldID, err := st.CreateSession(store.CreateSession{Label: "build", StartedAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatalf("create old session: %v", err)
	}
	newID, err := st.CreateSession(store.CreateSession{Label: "build", StartedAt: now})
	if err != nil {
		t.Fatalf("create new session: %v", err)
	}

	byID, err := resolveSession(st, strconv.FormatInt(oldID, 10))
	if err != nil {
		t.Fatalf("resolve by id: %v", err)
	}
	if byID.ID != oldID || byID.Label != "build" {
		t.Fatalf("by id = %+v, want id %d label build", byID, oldID)
	}

	byLabel, err := resolveSession(st, "build")
	if err != nil {
		t.Fatalf("resolve by label: %v", err)
	}
	if byLabel.ID != newID {
		t.Fatalf("by label id = %d, want newest id %d", byLabel.ID, newID)
	}
}

func TestResolveSessionReportsUsefulErrors(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	_, err = resolveSession(st, "123")
	if err == nil || !strings.Contains(err.Error(), "session 123 not found") {
		t.Fatalf("missing id error = %v", err)
	}
	_, err = resolveSession(st, "missing-label")
	if err == nil || !strings.Contains(err.Error(), `no session with label "missing-label"`) {
		t.Fatalf("missing label error = %v", err)
	}
}
