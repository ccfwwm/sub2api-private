package service

import (
	"context"
	"log/slog"
	"time"
)

type TextStickySnapshot struct {
	AccountID int64
	Revision  string
}

type textStickyWrite struct {
	snapshot  TextStickySnapshot
	accountID int64
	commit    func(context.Context, TextStickySnapshot) error
}

// ObserveTextSticky retains the version that the request used for routing.
// Subsequent lookups must not replace the expected version for a migration.
func ObserveTextSticky(ctx context.Context, key string, snapshot TextStickySnapshot) {
	route := textRouteFromContext(ctx)
	if route == nil || !route.enforce {
		return
	}
	if route.sticky == nil {
		route.sticky = make(map[string]textStickyWrite)
	}
	if _, exists := route.sticky[key]; !exists {
		route.sticky[key] = textStickyWrite{snapshot: snapshot}
	}
}

func StageTextSticky(ctx context.Context, key string, accountID int64,
	read func() (TextStickySnapshot, error), commit func(context.Context, TextStickySnapshot) error) error {
	route := textRouteFromContext(ctx)
	if route == nil || !route.enforce {
		return nil
	}
	if _, exists := route.sticky[key]; !exists {
		snapshot, err := read()
		if err != nil {
			return err
		}
		ObserveTextSticky(ctx, key, snapshot)
	}
	write := route.sticky[key]
	write.accountID = accountID
	write.commit = commit
	route.sticky[key] = write
	return nil
}

// InvalidateTextSticky defers a stale binding's replacement until a successful
// fallback. A preference-only search must not invalidate a valid binding.
func InvalidateTextSticky(ctx context.Context, key string) {
	route := textRouteFromContext(ctx)
	if route == nil || !route.enforce || ctx.Value(preferIndependentTextRouteKey{}) == true || route.failedOrigin != "" {
		return
	}
	if old := route.sticky[key].snapshot.AccountID; old > 0 {
		if route.coolingAccounts == nil {
			route.coolingAccounts = make(map[int64]bool)
		}
		route.coolingAccounts[old] = true
	}
}

func (r *textRouteRequest) commitSticky(ctx context.Context, accountID int64) {
	for _, write := range r.sticky {
		old := write.snapshot.AccountID
		if old > 0 && old != accountID && !r.coolingAccounts[old] {
			// Retain the warm owner for temporary spillover. The check also
			// protects legacy hash aliases when the primary key was absent.
			return
		}
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
	defer cancel()
	for _, write := range r.sticky {
		if write.accountID == accountID && write.commit != nil {
			if err := write.commit(writeCtx, write.snapshot); err != nil {
				slog.Debug("text_route_sticky_write_failed", "account_id", accountID, "error", err)
			}
		}
	}
}
