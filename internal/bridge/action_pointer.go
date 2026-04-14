package bridge

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

var scrollByCoordinateAction = ScrollByCoordinate
var scrollViewportCenter = func(ctx context.Context) (float64, float64, error) {
	var viewport struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({
		x: Math.max(1, Math.floor(window.innerWidth / 2)),
		y: Math.max(1, Math.floor(window.innerHeight / 2))
	})`, &viewport)); err != nil {
		return 0, 0, err
	}
	return viewport.X, viewport.Y, nil
}

// submitFormIfButton checks whether the target element is a submit button and,
// if so, uses requestSubmit() for a single-shot submission: constraint
// validation + submit event (so JS handlers run) + actual submission.
// Falls back to CDP click if the element is not a submit button or on error.
func submitFormIfButton(ctx context.Context, selector string) (bool, error) {
	var isSubmit bool
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return false;
			var tag = el.tagName.toLowerCase();
			var type = (el.type || '').toLowerCase();
			return (tag === 'button' && (type === 'submit' || type === '')) ||
			       (tag === 'input' && type === 'submit');
		})()
	`, selector), &isSubmit))
	if err != nil || !isSubmit {
		return false, err
	}
	// Fire full event chain via requestSubmit(el):
	// - runs constraint validation
	// - dispatches the submit event (so JS handlers like Odoo's fire)
	// - submits the form if nothing cancels it
	// One call, no double-fire (replaces manual dispatchEvent + form.submit).
	var submitted bool
	err = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return false;
			el.focus();
			var opts = {bubbles: true, cancelable: true};
			el.dispatchEvent(new MouseEvent('mousedown', opts));
			el.dispatchEvent(new MouseEvent('mouseup', opts));
			el.dispatchEvent(new MouseEvent('click', opts));
			var form = el.closest('form');
			if (form) { form.requestSubmit(el); }
			return true;
		})()
	`, selector), &submitted))
	return submitted, err
}

func (b *Bridge) actionClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Human {
		if req.Selector != "" {
			node, err := firstNodeBySelector(ctx, req.Selector)
			if err != nil {
				return nil, err
			}
			if err := HumanClickElement(ctx, req.TabID, b.TabManager, node.BackendNodeID); err != nil {
				return nil, err
			}
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true, "human": true}, nil
		}
		if req.NodeID > 0 {
			if err := HumanClickElement(ctx, req.TabID, b.TabManager, cdp.BackendNodeID(req.NodeID)); err != nil {
				return nil, err
			}
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true, "human": true}, nil
		}
		if req.HasXY {
			if _, err := HumanClick(ctx, req.TabID, b.TabManager, humanPointBox(humanPoint{X: req.X, Y: req.Y})); err != nil {
				return nil, err
			}
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true, "human": true}, nil
		}
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}

	var err error
	if req.Selector != "" {
		// For submit buttons, use requestSubmit() to fire constraint validation,
		// JS submit handlers, and actual submission in one shot (issue #411).
		submitted, subErr := submitFormIfButton(ctx, req.Selector)
		if subErr != nil {
			slog.Debug("submitFormIfButton failed, falling back to CDP click",
				"selector", req.Selector, "error", subErr)
		} else if submitted {
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true, "submitted": true}, nil
		}
		err = chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
	} else if req.NodeID > 0 {
		err = ClickByNodeID(ctx, req.NodeID)
	} else if req.HasXY {
		err = ClickByCoordinate(ctx, req.X, req.Y)
	} else {
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
	}
	if req.WaitNav {
		_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
	}
	return map[string]any{"clicked": true}, nil
}

func (b *Bridge) actionDoubleClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Human {
		var err error
		if req.Selector != "" {
			node, nodeErr := firstNodeBySelector(ctx, req.Selector)
			if nodeErr != nil {
				return nil, nodeErr
			}
			err = HumanDoubleClickElement(ctx, req.TabID, b.TabManager, node.BackendNodeID)
		} else if req.NodeID > 0 {
			err = HumanDoubleClickElement(ctx, req.TabID, b.TabManager, cdp.BackendNodeID(req.NodeID))
		} else if req.HasXY {
			_, err = HumanDoubleClick(ctx, req.TabID, b.TabManager, humanPointBox(humanPoint{X: req.X, Y: req.Y}))
		} else {
			return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"doubleclicked": true, "human": true}, nil
	}

	var err error
	if req.Selector != "" {
		err = chromedp.Run(ctx, chromedp.DoubleClick(req.Selector, chromedp.ByQuery))
	} else if req.NodeID > 0 {
		err = DoubleClickByNodeID(ctx, req.NodeID)
	} else if req.HasXY {
		err = DoubleClickByCoordinate(ctx, req.X, req.Y)
	} else {
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"doubleclicked": true}, nil
}

func (b *Bridge) actionHover(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Human {
		if req.NodeID > 0 {
			if err := HumanHoverElement(ctx, req.TabID, b.TabManager, cdp.BackendNodeID(req.NodeID)); err != nil {
				return nil, err
			}
			return map[string]any{"hovered": true, "human": true}, nil
		}
		if req.Selector != "" {
			node, err := firstNodeBySelector(ctx, req.Selector)
			if err != nil {
				return nil, err
			}
			if err := HumanHoverElement(ctx, req.TabID, b.TabManager, node.BackendNodeID); err != nil {
				return nil, err
			}
			return map[string]any{"hovered": true, "human": true}, nil
		}
		if req.HasXY {
			if _, err := HumanHover(ctx, req.TabID, b.TabManager, humanPointBox(humanPoint{X: req.X, Y: req.Y})); err != nil {
				return nil, err
			}
			return map[string]any{"hovered": true, "human": true}, nil
		}
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}

	if req.NodeID > 0 {
		return map[string]any{"hovered": true}, HoverByNodeID(ctx, req.NodeID)
	}
	if req.Selector != "" {
		return map[string]any{"hovered": true}, chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q)?.dispatchEvent(new MouseEvent('mouseover', {bubbles:true}))`, req.Selector), nil),
		)
	}
	if req.HasXY {
		return map[string]any{"hovered": true}, HoverByCoordinate(ctx, req.X, req.Y)
	}
	return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
}

func (b *Bridge) actionScroll(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Human {
		scrollX := req.ScrollX
		scrollY := req.ScrollY
		if scrollX == 0 && scrollY == 0 {
			scrollY = 800
		}

		if req.NodeID > 0 {
			if err := HumanScrollElement(ctx, req.TabID, b.TabManager, cdp.BackendNodeID(req.NodeID), scrollX, scrollY); err != nil {
				return nil, err
			}
			return map[string]any{"scrolled": true, "x": scrollX, "y": scrollY, "human": true}, nil
		}
		if req.Selector != "" {
			node, err := firstNodeBySelector(ctx, req.Selector)
			if err != nil {
				return nil, err
			}
			if err := HumanScrollElement(ctx, req.TabID, b.TabManager, node.BackendNodeID, scrollX, scrollY); err != nil {
				return nil, err
			}
			return map[string]any{"scrolled": true, "x": scrollX, "y": scrollY, "human": true}, nil
		}

		scrollTargetX := req.X
		scrollTargetY := req.Y
		if !req.HasXY {
			var err error
			scrollTargetX, scrollTargetY, err = scrollViewportCenter(ctx)
			if err != nil {
				return nil, fmt.Errorf("resolve scroll viewport center: %w", err)
			}
		}

		_, err := HumanScroll(ctx, req.TabID, b.TabManager, humanPointBox(humanPoint{X: scrollTargetX, Y: scrollTargetY}), scrollX, scrollY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"scrolled": true, "x": scrollX, "y": scrollY, "human": true}, nil
	}

	if req.NodeID > 0 {
		return map[string]any{"scrolled": true}, ScrollByNodeID(ctx, req.NodeID)
	}

	scrollX := req.ScrollX
	scrollY := req.ScrollY
	if scrollX == 0 && scrollY == 0 {
		scrollY = 800
	}

	scrollTargetX := req.X
	scrollTargetY := req.Y
	if !req.HasXY {
		var err error
		scrollTargetX, scrollTargetY, err = scrollViewportCenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve scroll viewport center: %w", err)
		}
	}

	return map[string]any{"scrolled": true, "x": scrollX, "y": scrollY},
		scrollByCoordinateAction(ctx, scrollTargetX, scrollTargetY, scrollX, scrollY)
}

func (b *Bridge) actionDrag(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.DragX == 0 && req.DragY == 0 {
		return nil, fmt.Errorf("dragX or dragY required for drag")
	}
	if req.Human {
		if req.NodeID > 0 {
			err := HumanDragElement(ctx, req.TabID, b.TabManager, cdp.BackendNodeID(req.NodeID), req.DragX, req.DragY)
			if err != nil {
				return nil, err
			}
			return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY, "human": true}, nil
		}
		if req.Selector != "" {
			node, err := firstNodeBySelector(ctx, req.Selector)
			if err != nil {
				return nil, err
			}
			err = HumanDragElement(ctx, req.TabID, b.TabManager, node.BackendNodeID, req.DragX, req.DragY)
			if err != nil {
				return nil, err
			}
			return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY, "human": true}, nil
		}
		return nil, fmt.Errorf("need selector, ref, or nodeId")
	}
	if req.NodeID > 0 {
		err := DragByNodeID(ctx, req.NodeID, req.DragX, req.DragY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		err = DragByNodeID(ctx, int64(node.BackendNodeID), req.DragX, req.DragY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
	}
	return nil, fmt.Errorf("need selector, ref, or nodeId")
}

func (b *Bridge) actionHumanClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	req.Human = true
	return b.actionClick(ctx, req)
}

func (b *Bridge) actionHumanDoubleClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	req.Human = true
	return b.actionDoubleClick(ctx, req)
}

func (b *Bridge) actionHumanDrag(ctx context.Context, req ActionRequest) (map[string]any, error) {
	req.Human = true
	return b.actionDrag(ctx, req)
}

func (b *Bridge) actionHumanHover(ctx context.Context, req ActionRequest) (map[string]any, error) {
	req.Human = true
	return b.actionHover(ctx, req)
}

func (b *Bridge) actionHumanScroll(ctx context.Context, req ActionRequest) (map[string]any, error) {
	req.Human = true
	return b.actionScroll(ctx, req)
}

func (b *Bridge) actionScrollIntoView(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		return ScrollIntoViewAndGetBox(ctx, req.NodeID)
	}
	if req.Selector != "" {
		nid, err := ResolveCSSToNodeID(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		return ScrollIntoViewAndGetBox(ctx, nid)
	}
	return nil, fmt.Errorf("need selector or ref")
}
