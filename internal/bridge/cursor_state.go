package bridge

import "time"

// CursorState tracks the last known synthetic pointer location for a tab.
// It is intentionally small so actions can cheaply reuse and update it.
type CursorState struct {
	X           float64
	Y           float64
	Valid       bool
	LastUpdated time.Time
	LastAction  string
}

func (tm *TabManager) GetCursorState(tabID string) (CursorState, bool) {
	if tm == nil {
		return CursorState{}, false
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	state, ok := tm.cursors[tabID]
	if !ok || state == nil {
		return CursorState{}, false
	}
	return *state, true
}

func (tm *TabManager) SetCursorState(tabID string, state CursorState) {
	if tm == nil || tabID == "" {
		return
	}
	state.Valid = true
	if state.LastUpdated.IsZero() {
		state.LastUpdated = time.Now()
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cursors[tabID] = &state
}

func (tm *TabManager) DeleteCursorState(tabID string) {
	if tm == nil || tabID == "" {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.cursors, tabID)
}
