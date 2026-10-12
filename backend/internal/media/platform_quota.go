package media

import (
	"context"
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"time"
)

// AdmissionQuota reads counters that include durable reservations, so a failed
// cache invalidation cannot let a legacy request spend an exhausted media hold.
// Returns the exhausted scope; expired windows remain owned by their old tasks.
func (s *Store) AdmissionQuota(ctx context.Context, keyID, subscriptionID int64) (string, error) {
	specs := []quotaSpec{
		{"key_total", "api_keys", "quota_used", "quota", "", keyID, 0, nil},
		{"key_5h", "api_keys", "usage_5h", "rate_limit_5h", "window_5h_start", keyID, 5 * time.Hour, nil},
		{"key_1d", "api_keys", "usage_1d", "rate_limit_1d", "window_1d_start", keyID, 24 * time.Hour, nil},
		{"key_7d", "api_keys", "usage_7d", "rate_limit_7d", "window_7d_start", keyID, 7 * 24 * time.Hour, nil},
	}
	now := time.Now()
	for _, q := range specs {
		if keyID == 0 {
			break
		}
		startColumn := "NULL::timestamptz"
		if q.start != "" {
			startColumn = q.start
		}
		var used, limit string
		var start sql.NullTime
		if err := s.DB.QueryRowContext(ctx, `SELECT `+q.used+`::text,`+q.limit+`::text,`+startColumn+` FROM api_keys WHERE id=$1 AND deleted_at IS NULL`, keyID).Scan(&used, &limit, &start); err != nil {
			return "", err
		}
		u, err := number(used)
		if err != nil {
			return "", err
		}
		l, err := number(limit)
		if err != nil {
			return "", err
		}
		current := q.period == 0 || start.Valid && now.Before(start.Time.Add(q.period))
		if current && l.IsPositive() && !u.LessThan(l) {
			return q.kind, nil
		}
	}
	if subscriptionID == 0 {
		return "", nil
	}
	var begins, expires time.Time
	if err := s.DB.QueryRowContext(ctx, `SELECT starts_at,expires_at FROM user_subscriptions WHERE id=$1 AND deleted_at IS NULL`, subscriptionID).Scan(&begins, &expires); err != nil {
		return "", err
	}
	for i, part := range []string{"daily", "weekly", "monthly"} {
		var used string
		var limit *string
		var start sql.NullTime
		if err := s.DB.QueryRowContext(ctx, `SELECT s.`+part+`_usage_usd::text,g.`+part+`_limit_usd::text,s.`+part+`_window_start FROM user_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.id=$1`, subscriptionID).Scan(&used, &limit, &start); err != nil {
			return "", err
		}
		if limit == nil || !start.Valid {
			continue
		}
		var current bool
		if part == "daily" {
			current = !expires.After(begins.AddDate(0, 0, 1)) || !start.Time.Before(timezone.StartOfDay(now))
		} else {
			period := []time.Duration{24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}[i]
			anchor := start.Time
			if anchor.Equal(timezone.StartOfDay(begins)) && anchor.Before(begins) {
				anchor = begins
			}
			next := anchor.Add(period)
			current = now.Before(next) || !next.Before(expires)
		}
		u, err := number(used)
		if err != nil {
			return "", err
		}
		l, err := number(*limit)
		if err != nil {
			return "", err
		}
		if current && !u.LessThan(l) {
			return "subscription_" + part, nil
		}
	}
	return "", nil
}

type PlatformQuota struct {
	Usage  map[string]string
	Limits map[string]*string
}

func (s *Store) PlatformQuota(ctx context.Context, userID int64, platform string) (*PlatformQuota, error) {
	q := &PlatformQuota{Usage: map[string]string{}, Limits: map[string]*string{}}
	var daily, weekly, monthly string
	var dl, wl, ml *string
	var ds, ws, ms sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT media_daily_usage_usd::text,media_weekly_usage_usd::text,media_monthly_usage_usd::text,daily_limit_usd::text,weekly_limit_usd::text,monthly_limit_usd::text,media_daily_window_start,media_weekly_window_start,media_monthly_window_start FROM user_platform_quotas WHERE user_id=$1 AND platform=$2 AND deleted_at IS NULL`, userID, platform).Scan(&daily, &weekly, &monthly, &dl, &wl, &ml, &ds, &ws, &ms)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	q.Usage["daily"] = daily
	q.Usage["weekly"] = weekly
	q.Usage["monthly"] = monthly
	if !ds.Valid || ds.Time.Before(timezone.StartOfDay(now)) {
		q.Usage["daily"] = "0"
	}
	if !ws.Valid || ws.Time.Before(timezone.StartOfWeek(now)) {
		q.Usage["weekly"] = "0"
	}
	if !ms.Valid || now.Sub(ms.Time) >= 30*24*time.Hour {
		q.Usage["monthly"] = "0"
	}
	q.Limits["daily"] = dl
	q.Limits["weekly"] = wl
	q.Limits["monthly"] = ml
	return q, nil
}
