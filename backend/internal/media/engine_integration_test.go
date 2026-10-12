//go:build integration

package media_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func pngFixture(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 4, 4))
	im.Set(0, 0, color.White)
	require.NoError(t, png.Encode(&buf, im))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestDisabledPublishedProfileRetainsMediaRoutingOwnership(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	snap := fixture(t, db, "10")
	cap := snap.Capability
	cap.Adapter = "v6k_video_gateway"
	cap.Operations = []string{"video"}
	cap.Version = "1"
	cap.ValidationStatus = "本项目实测通过"
	cap.VerificationEvidence = "isolated integration fixture"
	cap.Limits.MaxRequestBytes = ptr(int64(1024))
	cap.Limits.ObservationSeconds = ptr(3600)
	raw, err := json.Marshal(cap)
	require.NoError(t, err)
	ctx := context.Background()
	record, err := s.SaveConfig(ctx, "capability", cap.ID, 0, snap.Identity.UserID, raw, true)
	require.NoError(t, err)
	_, err = s.DisableConfig(ctx, "capability", cap.ID, record.Version, snap.Identity.UserID)
	require.NoError(t, err)
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("x", 32))
	require.NoError(t, err)
	engine := media.NewEngine(s, blobs, nil, nil)
	defer engine.Stop()
	active, err := engine.Capability(ctx, cap.Model, "video", snap.Identity)
	require.NoError(t, err)
	require.Nil(t, active)
	owned, err := engine.ManagedModel(ctx, cap.Model, "video", snap.Identity.GroupID)
	require.NoError(t, err)
	require.True(t, owned)
	owned, err = engine.ManagedModel(ctx, cap.Model, "video", snap.Identity.GroupID+999999)
	require.NoError(t, err)
	require.False(t, owned)
}
func TestEngineRecoversArtifactsAndSettlementWithoutResubmission(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "10")
	snap.Capability.MediaType = "image"
	snap.Capability.Limits.ObservationSeconds = ptr(3600)
	snap.Request.Operation = "image"
	snap.Sale.SuccessPolicy = "any_output"
	snap.SaleRule = media.Rule{Charges: []media.Charge{{Name: "generation", Unit: "output", Price: ptr("0.6"), QuantitySource: "successful_outputs", Trigger: "success", Refundable: true, Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "per_output"}}}}
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("x", 32))
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "crash-case", "hash", media.NewTaskID())
	require.NoError(t, err)
	res := map[string]any{"data": []any{map[string]any{"b64_json": pngFixture(t)}}}
	path, err := blobs.WriteJSON(task.ID, "result-1", res)
	require.NoError(t, err)
	require.NoError(t, s.Progress(ctx, task, media.UpstreamResult{Status: "completed", Quantities: map[string][]string{}}, path))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer upstream.Close()
	engine := media.NewEngine(s, blobs, func(id, group int64) (media.Account, error) {
		return media.Account{ID: id, BaseURL: upstream.URL, Key: "test", Allowed: true}, nil
	}, nil)
	defer engine.Stop()
	engine.Adapter.Client = media.SafeClient(true)
	require.NoError(t, engine.Process(ctx, task))
	require.Equal(t, 0, calls)
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "settled", task.Settlement)
	artifacts, err := s.Artifacts(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	require.Equal(t, []string{"1"}, task.Quantities["successful_outputs"])
	b, f := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.40000000", b)
	require.Equal(t, "0.00000000", f)
	file, mime, _, err := s.ArtifactFile(ctx, task.ID, artifacts[0].Key)
	require.NoError(t, err)
	_ = file.Close()
	require.Equal(t, "image/png", mime)
	var usageCount int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM usage_logs WHERE request_id=$1`, task.ID).Scan(&usageCount))
	require.Equal(t, 1, usageCount)
	require.NoError(t, s.VerifyCost(ctx, task.ID, "0.1", "supplier bill test", "cost1", task.Version, snap.Identity.UserID))
	require.NoError(t, s.VerifyCost(ctx, task.ID, "0.1", "supplier bill test", "cost1", task.Version, snap.Identity.UserID))
}

func TestEngineInvalidArtifactNeverChargesCustomer(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "5")
	snap.Capability.MediaType = "image"
	snap.Capability.Limits.ObservationSeconds = ptr(3600)
	snap.Request.Operation = "image"
	snap.Sale.SuccessPolicy = "any_output"
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("x", 32))
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	path, err := blobs.WriteJSON(task.ID, "result", map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("<html>not an image</html>"))}}})
	require.NoError(t, err)
	require.NoError(t, s.Progress(ctx, task, media.UpstreamResult{Status: "completed"}, path))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	engine := media.NewEngine(s, blobs, func(id, group int64) (media.Account, error) {
		return media.Account{ID: id, BaseURL: "https://example.invalid", Key: "test"}, nil
	}, nil)
	defer engine.Stop()
	require.NoError(t, engine.Process(ctx, task))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "pending_usage", task.Settlement)
	require.Equal(t, "0.00000000", task.Captured)
	artifacts, err := s.Artifacts(ctx, task.ID)
	require.NoError(t, err)
	require.Empty(t, artifacts)
}

func TestAcceptedFeeFailureRetainsOnlyBaseFee(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "10")
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	require.NoError(t, s.Settle(ctx, task.ID, media.Calculation{Total: "0.20000000"}, "accepted", nil))
	require.NoError(t, s.Progress(ctx, task, media.UpstreamResult{Status: "failed"}, ""))
	require.NoError(t, s.Settle(ctx, task.ID, media.Calculation{Total: "0.20000000"}, "failure", nil))
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "0.20000000", task.Accepted)
	require.Equal(t, "0.00000000", task.Captured)
	require.Equal(t, "0.80000000", task.Released)
	b, f := moneyState(t, db, snap.Identity.UserID)
	require.Equal(t, "9.80000000", b)
	require.Equal(t, "0.00000000", f)
	require.Error(t, s.Refund(ctx, task.ID, "r1", "0.2", "Cannot refund acceptance", task.Version, snap.Identity.UserID))
}

func TestQuotaWindowReleaseDoesNotCreditTheNewWindow(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "10")
	_, err := db.Exec(`UPDATE api_keys SET quota=10,rate_limit_5h=10 WHERE id=$1`, snap.Identity.KeyID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "", "", media.NewTaskID())
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE api_keys SET usage_5h=2,window_5h_start=now()+interval '1 hour' WHERE id=$1`, snap.Identity.KeyID)
	require.NoError(t, err)
	require.NoError(t, s.Progress(ctx, task, media.UpstreamResult{Status: "failed"}, ""))
	require.NoError(t, s.Settle(ctx, task.ID, media.Calculation{Total: "0.00000000"}, "failure", nil))
	var used, total string
	require.NoError(t, db.QueryRow(`SELECT usage_5h::text,quota_used::text FROM api_keys WHERE id=$1`, snap.Identity.KeyID).Scan(&used, &total))
	require.Equal(t, "2.00000000", used)
	require.Equal(t, "0.00000000", total)
}

func TestAsyncAdapterSubmitsOnceAndRecoversOnAnotherEngine(t *testing.T) {
	db := testDB(t)
	s := &media.Store{DB: db}
	ctx := context.Background()
	snap := fixture(t, db, "5")
	snap.Capability.MediaType = "image"
	snap.Capability.Limits.ObservationSeconds = ptr(3600)
	snap.Request.Operation = "image"
	snap.Sale.SuccessPolicy = "any_output"
	snap.SaleRule = media.Rule{Charges: []media.Charge{{Name: "generation", Unit: "request", Price: ptr("1"), QuantitySource: "task_count", Trigger: "success", Method: "linear", Rounding: media.Rounding{Mode: "none", Step: "1", Minimum: "0", Scope: "aggregate"}}}}
	submit, poll := 0, 0
	pngData := pngFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			submit++
			w.WriteHeader(202)
			json.NewEncoder(w).Encode(map[string]any{"id": "provider-once", "status": "queued"})
		} else {
			poll++
			json.NewEncoder(w).Encode(map[string]any{"id": "provider-once", "status": "completed", "data": []any{map[string]any{"b64_json": pngData}}})
		}
	}))
	defer upstream.Close()
	blobs, err := media.NewBlobStore(t.TempDir(), strings.Repeat("s", 32))
	require.NoError(t, err)
	id := media.NewTaskID()
	path, err := blobs.WriteJSON(id, "request", media.Request{Operation: "image", Parameters: map[string]any{"model": "mock", "prompt": "mock"}})
	require.NoError(t, err)
	snap.Request.Parameters = map[string]any{"request_blob": path}
	task, err := s.CreateTask(ctx, snap, snap.Capability.AccountIDs[0], "once", "hash", id)
	require.NoError(t, err)
	lookup := func(id, group int64) (media.Account, error) {
		return media.Account{ID: id, BaseURL: upstream.URL, Key: "test", Allowed: true}, nil
	}
	first := media.NewEngine(s, blobs, lookup, nil)
	first.Adapter.Client = media.SafeClient(true)
	task.State = "submitting"
	require.NoError(t, first.Process(ctx, task))
	first.Stop()
	require.Equal(t, 1, submit)
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "in_progress", task.State)
	second := media.NewEngine(s, blobs, lookup, nil)
	second.Adapter.Client = media.SafeClient(true)
	defer second.Stop()
	require.NoError(t, second.Process(ctx, task))
	require.Equal(t, 1, submit)
	require.Equal(t, 1, poll)
	task, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "settled", task.Settlement)
	_ = time.Second
}
