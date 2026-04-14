package bridge

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

var humanRand = rand.New(rand.NewSource(time.Now().UnixNano()))

const (
	humanCursorViewportMargin  = 60.0
	humanCursorOvershootChance = 0.65
	humanCursorOvershootRadius = 120.0
	humanCursorOvershootMin    = 30.0
	humanCursorOvershootSpread = 10.0
	humanCursorOvershootDist   = 500.0
	humanPointBoxSize          = 8.0
)

type humanPoint struct {
	X float64
	Y float64
}

type humanBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func SetHumanRandSeed(seed int64) {
	humanRand = rand.New(rand.NewSource(seed))
}

// Config allows injecting a custom random source for testing
type Config struct {
	Rand *rand.Rand
}

// getRand returns the configured random source or the global default
func (c *Config) getRand() *rand.Rand {
	if c != nil && c.Rand != nil {
		return c.Rand
	}
	return humanRand
}

func randomFloatInRange(rng *rand.Rand, min, max float64) float64 {
	if max <= min {
		return min
	}
	return min + rng.Float64()*(max-min)
}

func humanDistance(a, b humanPoint) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}

func humanClamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func humanFitts(distance, width float64) float64 {
	if width <= 0 {
		width = 100
	}
	return 2 * math.Log2(distance/width+1)
}

func humanRandomViewportPoint(ctx context.Context, rng *rand.Rand) (humanPoint, error) {
	var viewport struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({
		width: Math.max(1, window.innerWidth || document.documentElement.clientWidth || 1),
		height: Math.max(1, window.innerHeight || document.documentElement.clientHeight || 1)
	})`, &viewport)); err != nil {
		return humanPoint{}, err
	}
	marginX := math.Min(humanCursorViewportMargin, math.Max(1, viewport.Width/4))
	marginY := math.Min(humanCursorViewportMargin, math.Max(1, viewport.Height/4))
	return humanPoint{
		X: randomFloatInRange(rng, marginX, math.Max(marginX, viewport.Width-marginX)),
		Y: randomFloatInRange(rng, marginY, math.Max(marginY, viewport.Height-marginY)),
	}, nil
}

func humanTargetPoint(box humanBox, rng *rand.Rand) humanPoint {
	paddingX := math.Min(box.Width*0.2, 12)
	paddingY := math.Min(box.Height*0.2, 12)
	minX := box.X + paddingX
	maxX := box.X + math.Max(paddingX, box.Width-paddingX)
	minY := box.Y + paddingY
	maxY := box.Y + math.Max(paddingY, box.Height-paddingY)
	return humanPoint{
		X: randomFloatInRange(rng, minX, maxX),
		Y: randomFloatInRange(rng, minY, maxY),
	}
}

func humanPointBox(point humanPoint) humanBox {
	return humanBox{
		X:      point.X - humanPointBoxSize/2,
		Y:      point.Y - humanPointBoxSize/2,
		Width:  humanPointBoxSize,
		Height: humanPointBoxSize,
	}
}

func humanOvershootTarget(target humanPoint, radius float64, rng *rand.Rand) humanPoint {
	angle := rng.Float64() * 2 * math.Pi
	dist := radius * math.Sqrt(rng.Float64())
	return humanPoint{
		X: target.X + dist*math.Cos(angle),
		Y: target.Y + dist*math.Sin(angle),
	}
}

func humanBezierAnchors(start, end humanPoint, spread float64, rng *rand.Rand) (humanPoint, humanPoint) {
	side := 1.0
	if rng.Intn(2) == 0 {
		side = -1
	}
	dirX := end.X - start.X
	dirY := end.Y - start.Y
	length := math.Hypot(dirX, dirY)
	if length == 0 {
		return start, end
	}
	perpX := side * (dirY / length)
	perpY := side * (-dirX / length)
	calc := func(t float64) humanPoint {
		baseX := start.X + dirX*t
		baseY := start.Y + dirY*t
		offset := randomFloatInRange(rng, 0, spread)
		return humanPoint{
			X: baseX + perpX*offset,
			Y: baseY + perpY*offset,
		}
	}
	return calc(0.25), calc(0.75)
}

func humanBezierPoint(t float64, p0, p1, p2, p3 humanPoint) humanPoint {
	oneMinusT := 1 - t
	return humanPoint{
		X: oneMinusT*oneMinusT*oneMinusT*p0.X +
			3*oneMinusT*oneMinusT*t*p1.X +
			3*oneMinusT*t*t*p2.X +
			t*t*t*p3.X,
		Y: oneMinusT*oneMinusT*oneMinusT*p0.Y +
			3*oneMinusT*oneMinusT*t*p1.Y +
			3*oneMinusT*t*t*p2.Y +
			t*t*t*p3.Y,
	}
}

func humanPathSegment(start, end humanPoint, targetWidth, spreadOverride float64, rng *rand.Rand) []humanPoint {
	distance := humanDistance(start, end)
	if distance == 0 {
		return []humanPoint{end}
	}
	spread := spreadOverride
	if spread <= 0 {
		spread = humanClamp(distance, 2, 200)
	}
	if targetWidth <= 0 {
		targetWidth = 100
	}
	speed := randomFloatInRange(rng, 0.6, 1.4)
	steps := int(math.Ceil((math.Log2(humanFitts(distance, targetWidth)+1) + speed*25) * 3))
	if steps < 25 {
		steps = 25
	}
	if steps > 120 {
		steps = 120
	}
	c1, c2 := humanBezierAnchors(start, end, spread, rng)
	points := make([]humanPoint, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		point := humanBezierPoint(t, start, c1, c2, end)
		point.X = math.Max(0, point.X)
		point.Y = math.Max(0, point.Y)
		points = append(points, point)
	}
	return points
}

func humanPath(start, end humanPoint, targetBox humanBox, rng *rand.Rand) []humanPoint {
	distance := humanDistance(start, end)
	targetWidth := math.Max(targetBox.Width, targetBox.Height)
	if distance > humanCursorOvershootDist && rng.Float64() < humanCursorOvershootChance {
		radius := humanClamp(math.Min(targetWidth+20, humanCursorOvershootRadius), humanCursorOvershootMin, humanCursorOvershootRadius)
		overshoot := humanOvershootTarget(end, radius, rng)
		first := humanPathSegment(start, overshoot, targetWidth, 0, rng)
		second := humanPathSegment(overshoot, end, targetWidth, humanCursorOvershootSpread, rng)
		return append(first, second...)
	}
	return humanPathSegment(start, end, targetWidth, 0, rng)
}

func dispatchMousePath(ctx context.Context, points []humanPoint, rng *rand.Rand) error {
	for _, point := range points {
		x := point.X + randomFloatInRange(rng, -1, 1)
		y := point.Y + randomFloatInRange(rng, -1, 1)
		if err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
			}),
		); err != nil {
			return err
		}
		time.Sleep(time.Duration(8+rng.Intn(18)) * time.Millisecond)
	}
	return nil
}

func dispatchMouseEvent(ctx context.Context, eventType input.MouseType, point humanPoint, button input.MouseButton, clickCount int64, buttons int64) error {
	action := input.DispatchMouseEvent(eventType, point.X, point.Y)
	if button != "" {
		action = action.WithButton(button)
	}
	if clickCount > 0 {
		action = action.WithClickCount(clickCount)
	}
	if buttons > 0 {
		action = action.WithButtons(buttons)
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return action.Do(ctx)
	}))
}

func dispatchMouseWheelEvent(ctx context.Context, point humanPoint, deltaX, deltaY int) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
			"type":   "mouseWheel",
			"x":      point.X,
			"y":      point.Y,
			"deltaX": deltaX,
			"deltaY": deltaY,
		}, nil)
	}))
}

func humanSplitDelta(total int, parts int, rng *rand.Rand) []int {
	if total == 0 {
		return []int{0}
	}
	if parts < 1 {
		parts = 1
	}
	sign := 1
	if total < 0 {
		sign = -1
		total = -total
	}
	if parts > total {
		parts = total
	}
	if parts < 1 {
		parts = 1
	}
	if total < 40 || total < parts*8 {
		return []int{sign * total}
	}
	if parts == 1 {
		return []int{sign * total}
	}

	remaining := total
	chunks := make([]int, 0, parts)
	for i := 0; i < parts-1; i++ {
		remainingParts := parts - i
		ideal := remaining / remainingParts
		variance := int(math.Max(4, float64(ideal)/3))
		low := ideal - variance
		if low < 8 {
			low = 8
		}
		high := ideal + variance
		minRemaining := (remainingParts - 1) * 8
		if high > remaining-minRemaining {
			high = remaining - minRemaining
		}
		if high < low {
			high = low
		}
		chunk := low
		if high > low {
			chunk = low + rng.Intn(high-low+1)
		}
		chunks = append(chunks, sign*chunk)
		remaining -= chunk
	}
	chunks = append(chunks, sign*remaining)
	return chunks
}

func humanResolveStartPoint(ctx context.Context, tabID string, manager *TabManager, rng *rand.Rand) (humanPoint, error) {
	if manager != nil {
		if state, ok := manager.GetCursorState(tabID); ok && state.Valid {
			return humanPoint{X: state.X, Y: state.Y}, nil
		}
	}
	return humanRandomViewportPoint(ctx, rng)
}

func humanPersistCursor(tabID string, manager *TabManager, point humanPoint, action string) {
	if manager == nil || tabID == "" {
		return
	}
	manager.SetCursorState(tabID, CursorState{
		X:          point.X,
		Y:          point.Y,
		LastAction: action,
	})
}

func humanBoxForBackendNodeID(ctx context.Context, backendNodeID cdp.BackendNodeID) (humanBox, error) {
	var box *dom.BoxModel
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			box, err = dom.GetBoxModel().WithBackendNodeID(backendNodeID).Do(ctx)
			return err
		}),
	); err != nil {
		return humanBox{}, err
	}

	if len(box.Content) < 8 {
		return humanBox{}, fmt.Errorf("invalid box model")
	}

	xs := []float64{box.Content[0], box.Content[2], box.Content[4], box.Content[6]}
	ys := []float64{box.Content[1], box.Content[3], box.Content[5], box.Content[7]}
	minX, maxX := xs[0], xs[0]
	minY, maxY := ys[0], ys[0]
	for i := 1; i < len(xs); i++ {
		minX = math.Min(minX, xs[i])
		maxX = math.Max(maxX, xs[i])
		minY = math.Min(minY, ys[i])
		maxY = math.Max(maxY, ys[i])
	}
	return humanBox{
		X:      minX,
		Y:      minY,
		Width:  math.Max(1, maxX-minX),
		Height: math.Max(1, maxY-minY),
	}, nil
}

func humanPointerTargetFromNodeID(ctx context.Context, backendNodeID cdp.BackendNodeID) (humanBox, error) {
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": backendNodeID}, nil)
	})); err != nil {
		return humanBox{}, err
	}
	return humanBoxForBackendNodeID(ctx, backendNodeID)
}

func MouseMove(ctx context.Context, fromX, fromY, toX, toY float64) error {
	return dispatchMousePath(ctx, humanPathSegment(
		humanPoint{X: fromX, Y: fromY},
		humanPoint{X: toX, Y: toY},
		100,
		0,
		humanRand,
	), humanRand)
}

func Click(ctx context.Context, x, y float64) error {
	startOffsetX := (humanRand.Float64()-0.5)*200 + 50
	startOffsetY := (humanRand.Float64()-0.5)*200 + 50
	startX := x + startOffsetX
	startY := y + startOffsetY

	distance := math.Sqrt(startOffsetX*startOffsetX + startOffsetY*startOffsetY)
	if distance > 30 {
		if err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchMouseEvent(input.MouseMoved, startX, startY).Do(ctx)
			}),
		); err != nil {
			return err
		}

		if err := MouseMove(ctx, startX, startY, x, y); err != nil {
			return err
		}
	}

	time.Sleep(time.Duration(50+humanRand.Intn(150)) * time.Millisecond)

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, x, y).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	); err != nil {
		return err
	}

	time.Sleep(time.Duration(30+humanRand.Intn(90)) * time.Millisecond)

	releaseX := x + (humanRand.Float64()-0.5)*2
	releaseY := y + (humanRand.Float64()-0.5)*2

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, releaseX, releaseY).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	)
}

func HumanClick(ctx context.Context, tabID string, manager *TabManager, box humanBox) (humanPoint, error) {
	rng := humanRand
	start, err := humanResolveStartPoint(ctx, tabID, manager, rng)
	if err != nil {
		return humanPoint{}, err
	}

	target := humanTargetPoint(box, rng)
	if err := dispatchMousePath(ctx, humanPath(start, target, box, rng), rng); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(40+rng.Intn(120)) * time.Millisecond)

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, target.X, target.Y).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(20+rng.Intn(90)) * time.Millisecond)

	releaseX := target.X + randomFloatInRange(rng, -1, 1)
	releaseY := target.Y + randomFloatInRange(rng, -1, 1)
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, releaseX, releaseY).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	); err != nil {
		return humanPoint{}, err
	}

	final := humanPoint{X: releaseX, Y: releaseY}
	humanPersistCursor(tabID, manager, final, ActionHumanClick)
	return final, nil
}

func HumanDoubleClick(ctx context.Context, tabID string, manager *TabManager, box humanBox) (humanPoint, error) {
	rng := humanRand
	start, err := humanResolveStartPoint(ctx, tabID, manager, rng)
	if err != nil {
		return humanPoint{}, err
	}

	target := humanTargetPoint(box, rng)
	if err := dispatchMousePath(ctx, humanPath(start, target, box, rng), rng); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(30+rng.Intn(80)) * time.Millisecond)

	firstDown := target
	if err := dispatchMouseEvent(ctx, input.MousePressed, firstDown, input.Left, 1, 0); err != nil {
		return humanPoint{}, err
	}
	time.Sleep(time.Duration(18+rng.Intn(45)) * time.Millisecond)

	firstUp := humanPoint{
		X: target.X + randomFloatInRange(rng, -1, 1),
		Y: target.Y + randomFloatInRange(rng, -1, 1),
	}
	if err := dispatchMouseEvent(ctx, input.MouseReleased, firstUp, input.Left, 1, 0); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(40+rng.Intn(110)) * time.Millisecond)

	secondDown := humanPoint{
		X: target.X + randomFloatInRange(rng, -1.5, 1.5),
		Y: target.Y + randomFloatInRange(rng, -1.5, 1.5),
	}
	if err := dispatchMouseEvent(ctx, input.MousePressed, secondDown, input.Left, 2, 0); err != nil {
		return humanPoint{}, err
	}
	time.Sleep(time.Duration(15+rng.Intn(40)) * time.Millisecond)

	secondUp := humanPoint{
		X: secondDown.X + randomFloatInRange(rng, -1, 1),
		Y: secondDown.Y + randomFloatInRange(rng, -1, 1),
	}
	if err := dispatchMouseEvent(ctx, input.MouseReleased, secondUp, input.Left, 2, 0); err != nil {
		return humanPoint{}, err
	}

	humanPersistCursor(tabID, manager, secondUp, ActionDoubleClick)
	return secondUp, nil
}

func HumanDrag(ctx context.Context, tabID string, manager *TabManager, startBox humanBox, dx, dy int) (humanPoint, error) {
	rng := humanRand
	start, err := humanResolveStartPoint(ctx, tabID, manager, rng)
	if err != nil {
		return humanPoint{}, err
	}

	grab := humanTargetPoint(startBox, rng)
	if err := dispatchMousePath(ctx, humanPath(start, grab, startBox, rng), rng); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(35+rng.Intn(90)) * time.Millisecond)

	if err := dispatchMouseEvent(ctx, input.MousePressed, grab, input.Left, 1, 0); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(50+rng.Intn(120)) * time.Millisecond)

	end := humanPoint{
		X: math.Max(0, grab.X+float64(dx)),
		Y: math.Max(0, grab.Y+float64(dy)),
	}
	path := humanPathSegment(grab, end, math.Max(startBox.Width, startBox.Height), 0, rng)
	for _, point := range path {
		dragPoint := humanPoint{
			X: point.X + randomFloatInRange(rng, -0.8, 0.8),
			Y: point.Y + randomFloatInRange(rng, -0.8, 0.8),
		}
		if err := dispatchMouseEvent(ctx, input.MouseMoved, dragPoint, "", 0, 1); err != nil {
			return humanPoint{}, err
		}
		time.Sleep(time.Duration(10+rng.Intn(22)) * time.Millisecond)
	}

	time.Sleep(time.Duration(20+rng.Intn(70)) * time.Millisecond)

	release := humanPoint{
		X: end.X + randomFloatInRange(rng, -1, 1),
		Y: end.Y + randomFloatInRange(rng, -1, 1),
	}
	if err := dispatchMouseEvent(ctx, input.MouseReleased, release, input.Left, 1, 0); err != nil {
		return humanPoint{}, err
	}

	humanPersistCursor(tabID, manager, release, ActionDrag)
	return release, nil
}

// ClickElement clicks on an element identified by its backend DOM node ID.
// This uses the backendDOMNodeId from the accessibility tree, NOT a regular DOM nodeId.
func HumanClickElement(ctx context.Context, tabID string, manager *TabManager, backendNodeID cdp.BackendNodeID) error {
	box, err := humanBoxForBackendNodeID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	_, err = HumanClick(ctx, tabID, manager, box)
	return err
}

func HumanDoubleClickElement(ctx context.Context, tabID string, manager *TabManager, backendNodeID cdp.BackendNodeID) error {
	box, err := humanBoxForBackendNodeID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	_, err = HumanDoubleClick(ctx, tabID, manager, box)
	return err
}

func HumanDragElement(ctx context.Context, tabID string, manager *TabManager, backendNodeID cdp.BackendNodeID, dx, dy int) error {
	box, err := humanBoxForBackendNodeID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	_, err = HumanDrag(ctx, tabID, manager, box, dx, dy)
	return err
}

func HumanHover(ctx context.Context, tabID string, manager *TabManager, box humanBox) (humanPoint, error) {
	rng := humanRand
	start, err := humanResolveStartPoint(ctx, tabID, manager, rng)
	if err != nil {
		return humanPoint{}, err
	}

	target := humanTargetPoint(box, rng)
	if err := dispatchMousePath(ctx, humanPath(start, target, box, rng), rng); err != nil {
		return humanPoint{}, err
	}

	humanPersistCursor(tabID, manager, target, ActionHover)
	return target, nil
}

func HumanHoverElement(ctx context.Context, tabID string, manager *TabManager, backendNodeID cdp.BackendNodeID) error {
	box, err := humanPointerTargetFromNodeID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	_, err = HumanHover(ctx, tabID, manager, box)
	return err
}

func HumanScroll(ctx context.Context, tabID string, manager *TabManager, box humanBox, deltaX, deltaY int) (humanPoint, error) {
	rng := humanRand
	start, err := humanResolveStartPoint(ctx, tabID, manager, rng)
	if err != nil {
		return humanPoint{}, err
	}

	target := humanTargetPoint(box, rng)
	if err := dispatchMousePath(ctx, humanPath(start, target, box, rng), rng); err != nil {
		return humanPoint{}, err
	}

	time.Sleep(time.Duration(20+rng.Intn(60)) * time.Millisecond)

	steps := int(math.Max(math.Abs(float64(deltaX)), math.Abs(float64(deltaY))) / 180)
	if steps < 1 {
		steps = 1
	}
	if steps > 6 {
		steps = 6
	}
	xChunks := humanSplitDelta(deltaX, steps, rng)
	yChunks := humanSplitDelta(deltaY, steps, rng)

	for i := 0; i < steps; i++ {
		wheelPoint := humanPoint{
			X: target.X + randomFloatInRange(rng, -1.5, 1.5),
			Y: target.Y + randomFloatInRange(rng, -1.5, 1.5),
		}
		if err := dispatchMouseWheelEvent(ctx, wheelPoint, xChunks[i], yChunks[i]); err != nil {
			return humanPoint{}, err
		}
		if i < steps-1 {
			time.Sleep(time.Duration(18+rng.Intn(28)) * time.Millisecond)
		}
	}

	humanPersistCursor(tabID, manager, target, ActionScroll)
	return target, nil
}

func HumanScrollElement(ctx context.Context, tabID string, manager *TabManager, backendNodeID cdp.BackendNodeID, deltaX, deltaY int) error {
	box, err := humanPointerTargetFromNodeID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	_, err = HumanScroll(ctx, tabID, manager, box, deltaX, deltaY)
	return err
}

func ClickElement(ctx context.Context, backendNodeID cdp.BackendNodeID) error {
	return HumanClickElement(ctx, "", nil, backendNodeID)
}

func Type(text string, fast bool) []chromedp.Action {
	return TypeWithConfig(text, fast, nil)
}

// TypeWithConfig generates typing actions with optional custom random source
func TypeWithConfig(text string, fast bool, cfg *Config) []chromedp.Action {
	rng := cfg.getRand()
	actions := []chromedp.Action{}

	baseDelay := 80
	if fast {
		baseDelay = 40
	}

	chars := []rune(text)
	for i, char := range chars {
		actions = append(actions, chromedp.KeyEvent(string(char)))
		delay := baseDelay + rng.Intn(baseDelay/2)
		if rng.Float64() < 0.05 {
			delay += rng.Intn(500)
		}
		if i > 0 && chars[i-1] == char {
			delay = delay / 2
		}
		actions = append(actions, chromedp.Sleep(time.Duration(delay)*time.Millisecond))

		if rng.Float64() < 0.03 && i < len(chars)-1 {
			wrongChar := rune('a' + rng.Intn(26))
			actions = append(actions,
				chromedp.KeyEvent(string(wrongChar)),
				chromedp.Sleep(time.Duration(50+rng.Intn(100))*time.Millisecond),
				chromedp.KeyEvent("\b"),
				chromedp.Sleep(time.Duration(30+rng.Intn(70))*time.Millisecond),
			)
		}
	}
	return actions
}
