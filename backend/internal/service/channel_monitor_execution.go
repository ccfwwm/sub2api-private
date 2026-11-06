package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

type MonitorRun struct {
	RunID     string         `json:"run_id"`
	MonitorID int64          `json:"monitor_id"`
	State     string         `json:"state"`
	Results   []*CheckResult `json:"results"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
}
type monitorExecutionRepository interface {
	CreateMonitorRun(context.Context, *MonitorRun) error
	SaveMonitorRun(context.Context, *MonitorRun) error
	GetMonitorRun(context.Context, int64, string) (*MonitorRun, error)
	InterruptMonitorRuns(context.Context, time.Time) error
}
type monitorAtomicRepository interface {
	SaveCheckResults(context.Context, int64, []*ChannelMonitorHistoryRow, time.Time) error
}

func (s *ChannelMonitorService) StartMonitorRun(ctx context.Context, id int64) (*MonitorRun, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if !s.probeRuntime(ctx).ActiveProbesAllowed() {
		return nil, ErrChannelMonitorActiveProbesRetired
	}
	repo, ok := s.repo.(monitorExecutionRepository)
	if !ok {
		return nil, fmt.Errorf("durable monitor execution is unavailable")
	}
	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()
	if current, ok := s.activeRuns.Load(id); ok {
		if runID, valid := current.(string); valid {
			return repo.GetMonitorRun(ctx, id, runID)
		}
		return nil, fmt.Errorf("invalid active monitor run")
	}
	if !s.runsRecovered {
		if err := repo.InterruptMonitorRuns(ctx, time.Now()); err != nil {
			return nil, err
		}
		s.runsRecovered = true
	}
	v := &MonitorRun{RunID: uuid.NewString(), MonitorID: id, State: "running", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour), Results: []*CheckResult{}}
	if err := repo.CreateMonitorRun(ctx, v); err != nil {
		return nil, err
	}
	s.activeRuns.Store(id, v.RunID)
	copy := *v
	go func() {
		defer s.activeRuns.Delete(id)
		defer func() {
			if recovered := recover(); recovered != nil {
				v.State, v.Error = "failed", "Monitor execution interrupted; inspect the original task before resuming"
				saveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if saveErr := repo.SaveMonitorRun(saveCtx, v); saveErr != nil {
					slogMonitorSaveError(id, saveErr)
				}
			}
		}()
		results, err := s.RunCheck(context.Background(), id)
		v.Results = results
		v.State = "completed"
		if err != nil {
			v.State = "failed"
			v.Error = truncateMessage(err.Error())
		}
		saveCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if saveErr := repo.SaveMonitorRun(saveCtx, v); saveErr != nil {
			slogMonitorSaveError(id, saveErr)
		}
	}()
	// Do not share the mutable worker receipt with the request goroutine.
	return &copy, nil
}
func (s *ChannelMonitorService) GetMonitorRun(ctx context.Context, id int64, run string) (*MonitorRun, error) {
	if _, err := uuid.Parse(run); err != nil {
		return nil, infraerrors.BadRequest("INVALID_RUN_ID", "Invalid run ID")
	}
	repo, ok := s.repo.(monitorExecutionRepository)
	if !ok {
		return nil, fmt.Errorf("durable monitor execution is unavailable")
	}
	v, err := repo.GetMonitorRun(ctx, id, run)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.NotFound("MONITOR_RUN_NOT_FOUND", "Monitor run not found or expired")
	}
	if err != nil {
		return nil, err
	}
	if v.State == "running" {
		if _, ok := s.activeRuns.Load(id); !ok {
			v.State = "interrupted"
			v.Error = "Service restarted; resume the original task"
			if err = repo.SaveMonitorRun(ctx, v); err != nil {
				return nil, err
			}
		}
	}
	return v, nil
}

func monitorResultDir() string {
	root := os.Getenv("SUB2API_MONITOR_DATA_DIR")
	if root == "" {
		root = filepath.Join("data", "channel-monitor-tasks")
	}
	return filepath.Join(root, "results")
}
func (s *ChannelMonitorService) saveResultCheckpoint(id int64, results []*CheckResult) (string, error) {
	if err := os.MkdirAll(monitorResultDir(), 0700); err != nil {
		return "", err
	}
	b, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	sealed, err := s.encryptor.Encrypt(string(b))
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(monitorResultDir(), ".checkpoint-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err = f.Chmod(0600); err == nil {
		_, err = f.WriteString(sealed)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	path := filepath.Join(monitorResultDir(), fmt.Sprintf("%d-%s.state", id, uuid.NewString()))
	if err = os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, syncMonitorDir(monitorResultDir())
}
func (s *ChannelMonitorService) replayResultCheckpoints(ctx context.Context, m *ChannelMonitor) ([]*CheckResult, error) {
	files, err := filepath.Glob(filepath.Join(monitorResultDir(), fmt.Sprintf("%d-*.state", m.ID)))
	if err != nil {
		return nil, err
	}
	var recovered []*CheckResult
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		plain, e := s.encryptor.Decrypt(string(b))
		if e != nil {
			return nil, e
		}
		var results []*CheckResult
		if e = json.Unmarshal([]byte(plain), &results); e != nil {
			return nil, e
		}
		if e = s.writeCheckResults(ctx, m, results); e != nil {
			return nil, e
		}
		if e = s.acknowledgeMediaReceipts(m, results); e != nil {
			return nil, e
		}
		if e = os.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		recovered = append(recovered, results...)
	}
	return recovered, nil
}

func syncMonitorDir(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
func slogMonitorSaveError(id int64, err error) {
	slog.Error("channel_monitor: result persistence failed", "monitor_id", id, "error", err)
}
