package state

import (
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(store.List()); got != 0 {
		t.Fatalf("len(List()) = %d, want 0 for a fresh store", got)
	}

	store.Put(&Ranger{Name: "alpha", Status: StatusRunning, CreatedAt: time.Now().Add(-time.Hour)})
	store.Put(&Ranger{Name: "beta", Status: StatusDone, CreatedAt: time.Now()})
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load(root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	list := reloaded.List()
	if len(list) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(list))
	}
	if list[0].Name != "beta" {
		t.Errorf("List()[0] = %q, want the newest ranger first", list[0].Name)
	}

	if _, ok := reloaded.Get("alpha"); !ok {
		t.Error("Get(\"alpha\") missing after reload")
	}
	reloaded.Remove("alpha")
	if reloaded.Has("alpha") {
		t.Error("Has(\"alpha\") is true after Remove")
	}
}

func TestDurationUsesFinishedAt(t *testing.T) {
	start := time.Now().Add(-2 * time.Hour)
	end := start.Add(30 * time.Minute)
	r := &Ranger{CreatedAt: start, FinishedAt: &end}

	if got := r.Duration(); got != 30*time.Minute {
		t.Errorf("Duration() = %v, want 30m", got)
	}
	running := &Ranger{CreatedAt: start}
	if got := running.Duration(); got < 2*time.Hour {
		t.Errorf("Duration() = %v, want at least 2h for a running ranger", got)
	}
}

func TestStatusActive(t *testing.T) {
	if !StatusRunning.Active() {
		t.Error("StatusRunning.Active() = false")
	}
	for _, s := range []Status{StatusDone, StatusFailed, StatusStopped, StatusMerged} {
		if s.Active() {
			t.Errorf("%s.Active() = true, want false", s)
		}
	}
}
