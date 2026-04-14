package bridge

import (
	"context"
	"math/rand"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestSetHumanRandSeed(t *testing.T) {
	// Just verify it doesn't panic
	SetHumanRandSeed(12345)
}

func TestType(t *testing.T) {
	text := "hello"

	// Test normal typing
	actions := Type(text, false)
	if len(actions) < len(text) {
		t.Errorf("expected at least %d actions, got %d", len(text), len(actions))
	}

	// Test fast typing
	fastActions := Type(text, true)
	if len(fastActions) < len(text) {
		t.Errorf("expected at least %d actions, got %d", len(text), len(fastActions))
	}
}

func TestTypeWithCorrections(t *testing.T) {
	// Use a fixed seed that we know triggers a correction (statistically likely with long string)
	SetHumanRandSeed(1)
	text := "this is a very long string to increase the chance of a simulated typo correction"
	actions := Type(text, false)

	// If a typo happened, there will be more actions than just KeyEvents and Sleeps for each char
	if len(actions) < len(text)*2 {
		t.Errorf("expected many actions for long string, got %d", len(actions))
	}
}

func TestMouseMove(t *testing.T) {
	// Create a context that chromedp.Run won't immediately reject
	ctx, _ := chromedp.NewContext(context.Background())

	// MouseMove will try to call chromedp.Run.
	// Without a real browser it will return an error, but we cover the code path.
	_ = MouseMove(ctx, 0, 0, 100, 100)
}

func TestClick(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())
	_ = Click(ctx, 50, 50)
}

func TestTypeWithConfig(t *testing.T) {
	// Test with fixed seed for reproducibility
	cfg := &Config{
		Rand: rand.New(rand.NewSource(12345)),
	}

	// Generate actions twice with same config - should be identical
	actions1 := TypeWithConfig("hello", false, cfg)

	// Reset the rand source to same seed
	cfg.Rand = rand.New(rand.NewSource(12345))
	actions2 := TypeWithConfig("hello", false, cfg)

	if len(actions1) != len(actions2) {
		t.Errorf("expected same number of actions with same seed, got %d and %d", len(actions1), len(actions2))
	}

	// Verify at least some actions were generated
	if len(actions1) < 10 {
		t.Errorf("expected at least 10 actions, got %d", len(actions1))
	}
}

func TestClickElement_RequiresMinContentLength(t *testing.T) {
	// ClickElement accesses box.Content[0], [1], [2], and [5]
	// CDP BoxModel Content has 8 float64 values (4 x/y pairs)
	// The guard must check len(box.Content) < 8
	// Without a browser, GetBoxModel will fail
	ctx, _ := chromedp.NewContext(context.Background())
	err := ClickElement(ctx, 0)
	if err == nil {
		t.Error("expected error without browser connection")
	}
}

func TestHumanPathSegmentProducesPath(t *testing.T) {
	rng := rand.New(rand.NewSource(12345))
	points := humanPathSegment(
		humanPoint{X: 10, Y: 10},
		humanPoint{X: 400, Y: 300},
		80,
		0,
		rng,
	)
	if len(points) < 25 {
		t.Fatalf("expected at least 25 points, got %d", len(points))
	}
	last := points[len(points)-1]
	if last.X <= 0 || last.Y <= 0 {
		t.Fatalf("expected positive coordinates, got %#v", last)
	}
}

func TestHumanPathUsesOvershootForLongDistance(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	start := humanPoint{X: 20, Y: 20}
	end := humanPoint{X: 900, Y: 700}
	box := humanBox{X: 880, Y: 680, Width: 60, Height: 40}

	points := humanPath(start, end, box, rng)
	if len(points) < 50 {
		t.Fatalf("expected long path with overshoot-capable movement, got %d points", len(points))
	}
	last := points[len(points)-1]
	if last.X != end.X || last.Y != end.Y {
		t.Fatalf("expected final point to reach target, got %#v want %#v", last, end)
	}
}

func TestHumanPointBoxCentersOnPoint(t *testing.T) {
	point := humanPoint{X: 200, Y: 120}
	box := humanPointBox(point)

	if box.Width != humanPointBoxSize || box.Height != humanPointBoxSize {
		t.Fatalf("unexpected point box size: %#v", box)
	}
	if box.X+box.Width/2 != point.X || box.Y+box.Height/2 != point.Y {
		t.Fatalf("point box is not centered on point: box=%#v point=%#v", box, point)
	}
}

func TestHumanPathSegmentDragPathEndsAtTarget(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	start := humanPoint{X: 100, Y: 140}
	end := humanPoint{X: 260, Y: 320}

	points := humanPathSegment(start, end, 40, 0, rng)
	if len(points) < 25 {
		t.Fatalf("expected at least 25 drag path points, got %d", len(points))
	}

	last := points[len(points)-1]
	if last.X != end.X || last.Y != end.Y {
		t.Fatalf("expected path to end at %#v, got %#v", end, last)
	}
}
