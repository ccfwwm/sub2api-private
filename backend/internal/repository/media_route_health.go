package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/redis/go-redis/v9"
)

type mediaRouteHealth struct{ redis *redis.Client }

func NewMediaRouteHealth(rdb *redis.Client) media.RouteHealth {
	return &mediaRouteHealth{redis: rdb}
}

var checkMediaRoutes = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local wait = 0
for _, key in ipairs(KEYS) do
  local until_ms = tonumber(redis.call('GET', key) or '0')
  wait = math.max(wait, until_ms - now, redis.call('PTTL', key .. ':probe'))
end
if wait == 0 and ARGV[1] ~= '' then
  for _, key in ipairs(KEYS) do
    if redis.call('EXISTS', key) == 1 then
      redis.call('SET', key .. ':probe', ARGV[1], 'PX', 210000)
    end
  end
end
return wait
`)

func (h *mediaRouteHealth) Acquire(ctx context.Context, keys []string, owner string) (time.Duration, error) {
	ms, err := checkMediaRoutes.Run(ctx, h.redis, keys, owner).Int64()
	return time.Duration(ms) * time.Millisecond, err
}

func (h *mediaRouteHealth) RetryIn(ctx context.Context, keys []string) (time.Duration, error) {
	return h.Acquire(ctx, keys, "")
}

var rejectMediaRoute = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local delay = tonumber(ARGV[1])
local until_ms = tonumber(redis.call('GET', KEYS[1]) or '0')
if until_ms < now + delay then
  redis.call('SET', KEYS[1], tostring(now + delay), 'PX', delay + 86400000)
end
return 1
`)

func (h *mediaRouteHealth) Reject(ctx context.Context, key string, delay time.Duration) error {
	return rejectMediaRoute.Run(ctx, h.redis, []string{key}, delay.Milliseconds()).Err()
}

var finishMediaRoutes = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
for _, key in ipairs(KEYS) do
  local probe = key .. ':probe'
  if redis.call('GET', probe) == ARGV[1] then
    redis.call('DEL', probe)
    if tonumber(redis.call('GET', key) or '0') <= now then
      if ARGV[2] == '1' then
        redis.call('DEL', key)
      else
        redis.call('SET', key, tostring(now + 30000), 'PX', 86430000)
      end
    end
  end
end
return 1
`)

func (h *mediaRouteHealth) Finish(ctx context.Context, keys []string, owner string, accepted bool) error {
	status := 0
	if accepted {
		status = 1
	}
	return finishMediaRoutes.Run(ctx, h.redis, keys, owner, status).Err()
}

var observeMediaRoute = redis.NewScript(`
local key = KEYS[1]
local owner, outcome = ARGV[1], ARGV[2]
local probe = key .. ':probe'
local owns_probe = redis.call('GET', probe) == owner
if outcome == 'running' then
  if owns_probe then redis.call('PEXPIRE', probe, 210000) end
  return 0
end
local streak = key .. ':failures'
local open = redis.call('EXISTS', key) == 1
local delay = tonumber(ARGV[3])
if outcome == 'failed' or outcome == 'succeeded' then
  -- A poll can run again if the worker loses its lease before the SQL update.
  if not redis.call('SET', key .. ':observed:' .. owner, '1', 'NX', 'EX', 86400) then
    return 0
  end
end
if owns_probe then redis.call('DEL', probe) end
if outcome == 'succeeded' then
  if not open or owns_probe then redis.call('DEL', key, streak) end
  return 0
end
if open and not owns_probe then return 0 end
if outcome == 'failed' then
  if not open and redis.call('INCR', streak) < 3 then return 0 end
elseif owns_probe then
  delay = 30000
else
  return 0
end
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
redis.call('SET', key, tostring(now + delay), 'PX', delay + 86400000)
return 1
`)

func (h *mediaRouteHealth) Observe(ctx context.Context, key, owner string, outcome media.RouteOutcome, delay time.Duration) error {
	return observeMediaRoute.Run(ctx, h.redis, []string{key}, owner, string(outcome), delay.Milliseconds()).Err()
}
