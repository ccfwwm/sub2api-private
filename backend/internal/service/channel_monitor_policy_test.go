//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMonitorMessageUTF8JSONRoundtrip(t *testing.T) {
	for _, msg := range []string{strings.Repeat("中", 400), strings.Repeat("😀", 200), strings.Repeat("a", 485) + "中" + strings.Repeat("b", 100), string([]byte{0xff}) + strings.Repeat("错", 200)} {
		out := truncateMessage(msg)
		require.True(t, utf8.ValidString(out))
		require.LessOrEqual(t, len(out), 500)
		b, err := json.Marshal(out)
		require.NoError(t, err)
		var again string
		require.NoError(t, json.Unmarshal(b, &again))
		require.Equal(t, out, again)
		require.LessOrEqual(t, len(again), 500)
	}
}
func TestMonitorPolicyThresholdAndOverrides(t *testing.T) {
	prompt := "你好"
	normal := 95000
	timeout := 200000
	c := domain.MonitorProbeConfig{Models: map[string]domain.MonitorProbePolicy{"model": {Prompt: &prompt, NormalLatencyMs: &normal, RequestTimeoutMs: &timeout}}}
	p := resolveMonitorPolicy(c, "model", "image")
	require.Equal(t, 95*time.Second, p.NormalLatency)
	require.Equal(t, prompt, p.Prompt)
	require.Equal(t, 90*time.Second, resolveMonitorPolicy(c, "other", "image").NormalLatency)
	for _, tc := range []struct {
		duration time.Duration
		want     string
	}{{p.NormalLatency, MonitorStatusOperational}, {p.NormalLatency + time.Millisecond, MonitorStatusDegraded}} {
		r := finalizeOperationalOrDegraded(&CheckResult{}, tc.duration, int(tc.duration.Milliseconds()), p.NormalLatency)
		require.Equal(t, tc.want, r.Status)
	}
	require.NoError(t, validateMonitorPolicy(c, MonitorProviderOpenAI, "image", []string{"model"}, nil))
	timeout = 90000
	require.Error(t, validateMonitorPolicy(c, MonitorProviderOpenAI, "image", []string{"model"}, nil))
}

type receiptTestRepo struct {
	ChannelMonitorRepository
	monitor *ChannelMonitor
	fail    bool
	writes  int
	rows    []*ChannelMonitorHistoryRow
}

func (r *receiptTestRepo) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	copy := *r.monitor
	return &copy, nil
}
func (r *receiptTestRepo) SaveCheckResults(_ context.Context, _ int64, rows []*ChannelMonitorHistoryRow, _ time.Time) error {
	r.writes++
	if r.fail {
		return errors.New("database unavailable")
	}
	r.rows = rows
	return nil
}

func TestMonitorReceiptRetryDoesNotResubmit(t *testing.T) {
	t.Setenv("SUB2API_MONITOR_DATA_DIR", t.TempDir())
	swapMonitorHTTPClient(t)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"alive"}}]}`))
		}
	}))
	defer server.Close()
	prompt := "Say alive"
	repo := &receiptTestRepo{fail: true, monitor: &ChannelMonitor{ID: 123, Provider: MonitorProviderOpenAI, Endpoint: server.URL, APIKey: "OLD:test", PrimaryModel: "model", ProbeConfig: domain.MonitorProbeConfig{Defaults: domain.MonitorProbePolicy{Prompt: &prompt}}}}
	s := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	s.SetRuntimeReader(mediaCancellationRuntimeReader{})
	results, err := s.RunCheck(context.Background(), 123)
	require.Error(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 1, posts)
	repo.fail = false
	replayed, err := s.RunCheck(context.Background(), 123)
	require.NoError(t, err)
	require.Len(t, replayed, 1)
	require.Equal(t, 1, posts)
	require.Equal(t, results[0].SampleID, repo.rows[0].SampleID)
}
