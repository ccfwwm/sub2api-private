package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebsiteReferencesAndExtensionsReachUpstreamDespiteCatalogLimits(t *testing.T) {
	for _, format := range []string{"v6k_url", "yunfei_multimodal"} {
		t.Run(format, func(t *testing.T) {
			cap := videoFormatCapability(format)
			cap.Materials = map[string]Material{"image": {MaxCount: pointer(1)}}
			base := "https://zerowall.chengxunkeji.cn/webfiles/"
			images := []any{}
			for _, name := range []string{"zerowall-icon.png", "01-medical-research-workspace.png", "02-singlecell-virtual-knockout.png", "03-genetics-brand-finale.png"} {
				images = append(images, map[string]any{"url": base + name, "weight": 0.7})
			}
			videos := []any{map[string]any{"url": base + "zerowallsciencevideo.mp4", "duration_seconds": float64(99), "future_motion": "preserve"}}
			req, err := Normalize(cap, "video", map[string]any{"model": cap.Model, "prompt": "animate", "duration": 5, "images": images, "videos": videos, "quality": "future", "future_options": map[string]any{"enabled": true}})
			require.NoError(t, err)
			var actual []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actual, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
			}))
			defer server.Close()
			var receipts []Submission
			a := Adapter{Client: SafeClient(true)}
			result, err := a.Submit(context.Background(), cap, req, Account{BaseURL: server.URL, Key: "fixture-key", ObserveSubmission: func(receipt Submission) error { receipts = append(receipts, receipt); return nil }})
			require.NoError(t, err)
			require.True(t, result.Accepted)
			var body map[string]any
			require.NoError(t, json.Unmarshal(actual, &body))
			prefix := ""
			if format == "yunfei_multimodal" {
				prefix = "reference_"
			}
			require.Equal(t, images, body[prefix+"images"])
			require.EqualValues(t, videos, body[prefix+"videos"])
			require.Equal(t, map[string]any{"enabled": true}, body["future_options"])
			require.Equal(t, "future", body["quality"])
			require.GreaterOrEqual(t, len(receipts), 3)
			require.Equal(t, "prepared", receipts[0].Stage)
			require.Equal(t, "accepted", receipts[len(receipts)-1].Stage)
			receipt := receipts[len(receipts)-1]
			require.Equal(t, 4, receipt.References["image"])
			require.Equal(t, 1, receipt.References["video"])
			require.Equal(t, NewSubmission("/v1/videos", actual, body, req.MaterialRefs, "").RequestHash, receipt.RequestHash)
			require.Equal(t, cap.Model, req.Incoming["model"])
		})
	}
}

func TestSubmissionReceiptRedactsExtensionsAndDoesNotInventHistory(t *testing.T) {
	input := map[string]any{"custom": map[string]any{"token": "secret", "label": "https://user:password@example.com/reference?signature=secret"}, "prompt": "Bearer secret sk-hidden", "future_option": true, "b64_json": strings.Repeat("A", 200)}
	raw, _ := json.Marshal(SafeReviewValue(input, []string{"secret"}, 0))
	require.NotContains(t, string(raw), "password")
	require.NotContains(t, string(raw), "signature=")
	require.NotContains(t, string(raw), "sk-hidden")
	require.Contains(t, string(raw), "future_option")
	require.Contains(t, string(raw), "redacted")
	e := Engine{}
	require.Nil(t, e.ReadSubmission("mt_history"))
}

func TestPublicTaskProjectionOmitsSubmissionAndPrivateRuntimeData(t *testing.T) {
	const rawURL = "https://media.example.com/reference.png?signature=private"
	const encoded = "data:image/png;base64,cHJpdmF0ZS1pbmxpbmU="
	internal := StoredTask{
		ID: "mt_public_projection", State: "in_progress", Settlement: "held",
		Snapshot: Snapshot{
			Capability: Capability{
				Model: "public-model", UpstreamModel: "private-upstream-model",
				Template: &ProtocolTemplate{ID: "private-template-id", Auth: TemplateAuth{Mode: "header", Name: "X-Private-Auth"}},
			},
			Request: Request{MaterialRefs: map[string][]any{"image": {map[string]any{"url": rawURL}}}},
		},
	}
	public := publicTaskValue(internal, []Artifact{{Key: "image1", Type: "image", MIME: "image/png", Source: rawURL}})
	public.Submission = &Submission{
		Path: "/private/submit", Request: map[string]any{"images": []any{encoded}, "credential": "private-credential"},
	}
	raw, err := json.Marshal(public)
	require.NoError(t, err)
	body := string(raw)
	for _, forbidden := range []string{rawURL, encoded, "private-upstream-model", "private-template-id", "X-Private-Auth", "private-credential", "/private/submit"} {
		require.NotContains(t, body, forbidden)
	}
	require.Contains(t, body, "public-model")
}

func TestNativeArkPreservesReferenceAttributesAndOptionalParameters(t *testing.T) {
	cap := videoFormatCapability("legacy")
	cap.Adapter = "ark_native"
	cap.Materials = nil
	req, err := Normalize(cap, "video", map[string]any{"model": cap.Model, "prompt": "animate", "seconds": 5, "future_option": map[string]any{"enabled": true}, "images": []any{map[string]any{"url": "https://media.example.com/ref.png", "role": "reference", "weight": 0.7}}})
	require.NoError(t, err)
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"native-task","status":"queued"}`))
	}))
	defer server.Close()
	_, err = (&Adapter{Client: SafeClient(true)}).Submit(context.Background(), cap, req, Account{BaseURL: server.URL, Key: "fixture-key"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"enabled": true}, body["future_option"])
	require.Contains(t, body["content"], map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://media.example.com/ref.png"}, "role": "reference", "weight": 0.7})
}

func TestSubmissionReceiptSurvivesEngineRestartWithoutLeakingRawInput(t *testing.T) {
	root := t.TempDir()
	blobs, err := NewBlobStore(root, strings.Repeat("s", 32))
	require.NoError(t, err)
	body := map[string]any{"model": "selected-model", "images": []any{map[string]any{"url": "https://media.example.com/a.png?signature=private"}}, "future_token": "private"}
	raw, _ := json.Marshal(body)
	receipt := NewSubmission("/v1/videos", raw, body, map[string][]any{"image": body["images"].([]any)}, "private")
	receipt.Stage = "attempted"
	path, err := blobs.WriteJSON("mt_receipt_restart", "submission", receipt)
	require.NoError(t, err)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(stored), "selected-model")
	require.NotContains(t, string(stored), "signature")
	restarted, err := NewBlobStore(root, strings.Repeat("s", 32))
	require.NoError(t, err)
	got := (&Engine{Blobs: restarted}).ReadSubmission("mt_receipt_restart")
	require.NotNil(t, got)
	require.Equal(t, "attempted", got.Stage)
	require.Equal(t, receipt.RequestHash, got.RequestHash)
	require.Equal(t, 1, got.References["image"])
	require.Nil(t, (&Engine{Blobs: restarted}).ReadSubmission("mt_unrecorded_history"))
}

func TestSubmissionNeverConfusesAttemptWithAcceptance(t *testing.T) {
	for _, status := range []int{200, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"code":"REF_UNSUPPORTED","message":"Bearer fixture-key https://media.example.com/error?secret=private"}}`))
			}))
			defer server.Close()
			cap := videoFormatCapability("v6k_url")
			req, err := Normalize(cap, "video", map[string]any{"model": cap.Model, "prompt": "animate", "seconds": 5})
			require.NoError(t, err)
			var receipt Submission
			result, err := (&Adapter{Client: SafeClient(true)}).Submit(context.Background(), cap, req, Account{BaseURL: server.URL, Key: "fixture-key", ObserveSubmission: func(v Submission) error { receipt = v; return nil }})
			require.NoError(t, err)
			require.False(t, result.Accepted)
			require.Equal(t, "attempted", receipt.Stage)
			raw, _ := json.Marshal(receipt)
			require.NotContains(t, string(raw), "fixture-key")
			require.NotContains(t, string(raw), "secret=private")
			if status == 422 {
				require.Contains(t, result.Diagnostic, "REF_UNSUPPORTED")
				require.NotContains(t, result.Diagnostic, "fixture-key")
			}
		})
	}
}
