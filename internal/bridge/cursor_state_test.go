package bridge

import "testing"

func TestCursorStateLifecycle(t *testing.T) {
	tm := &TabManager{cursors: make(map[string]*CursorState)}

	if _, ok := tm.GetCursorState("tab1"); ok {
		t.Fatal("expected missing cursor state")
	}

	tm.SetCursorState("tab1", CursorState{
		X:          120.5,
		Y:          240.25,
		LastAction: ActionHumanClick,
	})

	state, ok := tm.GetCursorState("tab1")
	if !ok {
		t.Fatal("expected cursor state")
	}
	if !state.Valid {
		t.Fatal("expected valid cursor state")
	}
	if state.X != 120.5 || state.Y != 240.25 {
		t.Fatalf("unexpected cursor coordinates: %#v", state)
	}
	if state.LastAction != ActionHumanClick {
		t.Fatalf("unexpected last action: %s", state.LastAction)
	}

	tm.DeleteCursorState("tab1")
	if _, ok := tm.GetCursorState("tab1"); ok {
		t.Fatal("expected cursor state to be deleted")
	}
}
