package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SubmissionRoutes struct {
	Candidates []SubmissionRoute   `json:"candidates"`
	Attempts   []SubmissionAttempt `json:"attempts,omitempty"`
}

type SubmissionRoute struct {
	AccountID      int64      `json:"account_id"`
	Origin         string     `json:"origin"`
	Capability     Capability `json:"capability"`
	Cost           *PriceCard `json:"cost,omitempty"`
	CostRule       *Rule      `json:"cost_rule,omitempty"`
	CostMultiplier string     `json:"cost_multiplier"`
}

type SubmissionAttempt struct {
	AccountID int64     `json:"account_id"`
	Epoch     int64     `json:"lease_epoch"`
	StartedAt time.Time `json:"started_at"`
	Outcome   string    `json:"outcome"`
	Status    int       `json:"upstream_status,omitempty"`
	Code      string    `json:"upstream_code,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	RequestID string    `json:"upstream_request_id,omitempty"`
}

func (r SubmissionRoute) target(group int64, operation string) RouteTarget {
	return RouteTarget{GroupID: group, AccountID: r.AccountID, Origin: r.Origin, Model: r.Capability.Model, Operation: operation}
}

func (r SubmissionRoute) assign(snapshot *Snapshot) {
	snapshot.Capability = r.Capability
	snapshot.Cost, snapshot.CostRule, snapshot.CostMultiplier = r.Cost, r.CostRule, r.CostMultiplier
}

func freezeSubmissionRoute(snapshot Snapshot, account Account) SubmissionRoute {
	return SubmissionRoute{
		AccountID: account.ID, Origin: RouteOrigin(account.BaseURL, account.Platform), Capability: snapshot.Capability,
		Cost: snapshot.Cost, CostRule: snapshot.CostRule, CostMultiplier: snapshot.CostMultiplier,
	}
}

func sameJSON(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func sameSubmissionRequest(first, candidate Request) bool {
	if first.Duration != candidate.Duration || first.InputImages != candidate.InputImages ||
		!sameJSON(first.Conditions, candidate.Conditions) || !sameJSON(first.Quantities, candidate.Quantities) ||
		!sameJSON(first.MaterialRefs, candidate.MaterialRefs) {
		return false
	}
	semantic := func(req Request) map[string]any {
		p := cloneObject(req.Parameters)
		delete(p, "model")
		return p
	}
	return sameJSON(semantic(first), semantic(candidate))
}

func candidateInput(req Request) map[string]any {
	input := cloneObject(req.Incoming)
	delete(input, "public_profile_id")
	return input
}

func sameSale(first, candidate Snapshot) bool {
	return first.Maximum == candidate.Maximum &&
		first.Identity.Multiplier == candidate.Identity.Multiplier &&
		first.Identity.TimeMultiplier == candidate.Identity.TimeMultiplier &&
		sameJSON(first.Sale, candidate.Sale) && sameJSON(first.SaleRule, candidate.SaleRule)
}

func compatibleOutput(first, candidate Capability) bool {
	if first.MediaType != candidate.MediaType || !sameJSON(first.OutputFormats, candidate.OutputFormats) {
		return false
	}
	if first.Template == nil || candidate.Template == nil {
		return first.Template == nil && candidate.Template == nil
	}
	return canonicalMediaMIME(first.Template.Response.MIME) == canonicalMediaMIME(candidate.Template.Response.MIME) &&
		(first.Template.Response.PCMToWAV != nil) == (candidate.Template.Response.PCMToWAV != nil)
}

func (e *Engine) createRoutedTask(ctx context.Context, cap Capability, operation string, input map[string]any, uploads []Upload, identity Identity, idem, hash string) (StoredTask, error) {
	caps, cards, err := e.configuration(ctx, identity.GroupID)
	if err != nil {
		return StoredTask{}, err
	}
	if cap.Template != nil && !slices.ContainsFunc(caps, func(current Capability) bool {
		return current.ID == cap.ID && current.Version == cap.Version && current.Enabled
	}) {
		return StoredTask{}, fail(409, "estimate_configuration_changed", "Template, binding or account configuration changed; refresh estimate")
	}
	req, err := e.PrepareRequest(ctx, cap, operation, input, uploads)
	if err != nil {
		return StoredTask{}, err
	}
	req.Incoming, req.Uploads = cloneObject(input), uploads
	now := time.Now()
	var primary Account
	for _, id := range cap.AccountIDs {
		if account, lookupErr := e.Lookup(id, identity.GroupID); lookupErr == nil && account.Allowed {
			primary = account
			break
		}
	}
	if primary.ID == 0 {
		return StoredTask{}, fail(503, "channel_unavailable", "No configured account is currently eligible")
	}
	snapshot, _, err := snapshotForAccount(cards, cap, req, identity, primary, now)
	if err != nil {
		return StoredTask{}, err
	}
	snapshot.Routing = &SubmissionRoutes{Candidates: []SubmissionRoute{freezeSubmissionRoute(snapshot, primary)}}
	seen := map[int64]bool{primary.ID: true}
	baseInput := candidateInput(req)
	for _, alternative := range caps {
		if !alternative.Matches(cap.Model, operation, identity.GroupID) || !compatibleOutput(cap, alternative) ||
			!mediaRequestFits(alternative, baseInput, uploads) {
			continue
		}
		prepared, normalizeErr := NormalizeUploads(alternative, operation, baseInput, uploads)
		if normalizeErr != nil {
			continue
		}
		prepared.Quantities["task_count"] = []string{"1"}
		for _, kind := range []string{"accepted_input_audio_duration", "accepted_input_video_duration"} {
			if measured, present := req.Quantities[kind]; present {
				prepared.Quantities[kind] = measured
			}
		}
		if measured, present := req.Quantities["accepted_input_images"]; present && prepared.InputImages == req.InputImages {
			prepared.Quantities["accepted_input_images"] = measured
		}
		if !sameSubmissionRequest(req, prepared) || !fitsMediaSpecifications(alternative, prepared) {
			continue
		}
		for _, id := range alternative.AccountIDs {
			if seen[id] {
				continue
			}
			account, lookupErr := e.Lookup(id, identity.GroupID)
			if lookupErr != nil || !account.Allowed {
				continue
			}
			quote, _, quoteErr := snapshotForAccount(cards, alternative, prepared, identity, account, now)
			if quoteErr == nil && sameSale(snapshot, quote) {
				snapshot.Routing.Candidates = append(snapshot.Routing.Candidates, freezeSubmissionRoute(quote, account))
				seen[id] = true
			}
		}
	}
	id := NewTaskID()
	var admission error
	var retry time.Duration
	for _, route := range snapshot.Routing.Candidates {
		if delay := e.Routes.RetryIn(ctx, route.target(identity.GroupID, Operation(operation))); delay > 0 {
			retry = earlierRetry(retry, delay)
			continue
		}
		prepared, prepareErr := e.prepareSubmissionRoute(ctx, req, snapshot.Routing.Candidates[0], route)
		if prepareErr != nil {
			continue
		}
		next := snapshot
		route.assign(&next)
		task, createErr := e.persistTask(ctx, id, fmt.Sprintf("request-%d", route.AccountID), next, prepared, route.AccountID, idem, hash)
		if createErr == nil || !mediaAdmissionFailure(createErr) {
			return task, createErr
		}
		admission = createErr
		_ = os.Remove(filepath.Join(e.Blobs.Root, id, fmt.Sprintf("request-%d", route.AccountID)))
	}
	if admission != nil {
		return StoredTask{}, admission
	}
	if retry > 0 {
		return StoredTask{}, &Error{Status: 429, Code: "media_routes_cooling_down", Message: "Compatible media routes are cooling down", RetryAfter: retrySeconds(retry)}
	}
	return StoredTask{}, fail(503, "channel_unavailable", "No compatible media route is available")
}

func mediaRequestFits(cap Capability, input map[string]any, uploads []Upload) bool {
	raw, err := json.Marshal(input)
	if err != nil || cap.Limits.MaxRequestBytes == nil {
		return false
	}
	size := int64(len(raw))
	for _, file := range uploads {
		size += int64(len(file.Data))
	}
	return size <= *cap.Limits.MaxRequestBytes
}

func fitsMediaSpecifications(cap Capability, request Request) bool {
	for key, allowed := range map[string][]string{
		"resolution": cap.Resolutions, "ratio": cap.AspectRatios, "size": cap.Sizes,
		"quality": cap.Qualities, "background": cap.Backgrounds, "output_format": cap.OutputFormats,
	} {
		if value := stringValue(request.Conditions[key]); value != "" && len(allowed) > 0 &&
			!slices.ContainsFunc(allowed, func(candidate string) bool { return strings.EqualFold(candidate, value) }) {
			return false
		}
	}
	if request.Duration == "" {
		return true
	}
	duration, err := strconv.Atoi(request.Duration)
	if err != nil {
		return false
	}
	switch cap.Duration.Mode {
	case "fixed", "enum":
		return len(cap.Duration.Values) == 0 || slices.Contains(cap.Duration.Values, duration)
	case "range":
		return (cap.Duration.Min == nil || duration >= *cap.Duration.Min) && (cap.Duration.Max == nil || duration <= *cap.Duration.Max)
	}
	return true
}

func (e *Engine) prepareSubmissionRoute(ctx context.Context, original Request, current, route SubmissionRoute) (Request, error) {
	if current.AccountID == route.AccountID && current.Capability.ID == route.Capability.ID {
		return original, nil
	}
	next, err := e.PrepareRequest(ctx, route.Capability, original.Operation, candidateInput(original), original.Uploads)
	if err != nil {
		return next, err
	}
	if !sameSubmissionRequest(original, next) {
		return next, fail(409, "media_profile_incompatible", "Backup route changes request specifications")
	}
	next.Incoming, next.Uploads = original.Incoming, original.Uploads
	return next, nil
}

func mediaAdmissionFailure(err error) bool {
	var problem *Error
	return errors.As(err, &problem) &&
		(problem.Code == "account_concurrency_exhausted" || problem.Code == "media_account_unavailable")
}

func earlierRetry(current, next time.Duration) time.Duration {
	if current <= 0 || next < current {
		return next
	}
	return current
}

func retrySeconds(delay time.Duration) int {
	return int((delay + time.Second - 1) / time.Second)
}

func (e *Engine) processRoutedSubmission(ctx context.Context, task StoredTask) error {
	routing := task.Snapshot.Routing
	index := slices.IndexFunc(routing.Candidates, func(route SubmissionRoute) bool { return route.AccountID == task.AccountID })
	if index < 0 {
		return e.deferTask(ctx, task, "Missing submission route; inspect task snapshot")
	}
	route := routing.Candidates[index]
	var request Request
	if err := e.Blobs.ReadJSON(stringValue(task.Snapshot.Request.Parameters["request_blob"]), &request); err != nil {
		return e.deferTask(ctx, task, "Durable request unavailable; inspect storage")
	}
	account, err := e.Lookup(task.AccountID, task.Identity.GroupID)
	target := route.target(task.Identity.GroupID, request.Operation)
	owner := fmt.Sprintf("%s:%d", task.ID, task.Epoch)
	if err != nil || !account.Allowed || account.Key == "" || route.Origin != RouteOrigin(account.BaseURL, account.Platform) ||
		len(routing.Attempts) >= MaxSubmissionAttempts {
		return e.alternateOrFinish(ctx, task, route, request, UpstreamResult{Status: "failed", Diagnostic: "Media route is no longer eligible"})
	}
	if delay := e.Routes.Acquire(ctx, target, owner); delay > 0 {
		return e.alternateOrFinish(ctx, task, route, request, UpstreamResult{Status: "failed", UpstreamStatusCode: http.StatusTooManyRequests,
			Diagnostic: "Media route is cooling down", RetryAfter: delay})
	}
	routing.Attempts = append(routing.Attempts, SubmissionAttempt{AccountID: account.ID, Epoch: task.Epoch, StartedAt: time.Now().UTC(), Outcome: "submitting"})
	if err = e.Store.beginRoutedSubmission(ctx, task); err != nil {
		e.Routes.Finish(ctx, target, owner, "", 0, false)
		return err
	}
	receiptKey := "submission-" + strconv.FormatInt(task.Epoch, 10)
	account.RouteSubmission = true
	account.IdempotencyKey = task.ID
	var submission Submission
	var receiptMu sync.Mutex
	account.ObserveSubmission = func(receipt Submission) error {
		receiptMu.Lock()
		defer receiptMu.Unlock()
		// Preserve the prepared payload when the adapter adds a transport receipt.
		if receipt.Path == "" && receipt.Stage != "prepared" {
			if submission.Stage != "accepted" {
				submission.Stage = receipt.Stage
			}
			if receipt.ResponseStatusCode != 0 {
				submission.ResponseStatusCode = receipt.ResponseStatusCode
				submission.ResponseContentType = receipt.ResponseContentType
				submission.Response = receipt.Response
			}
		} else {
			submission = receipt
		}
		_, writeErr := e.Blobs.WriteJSON(task.ID, receiptKey, submission)
		return writeErr
	}
	result, err := e.Adapter.Submit(ctx, task.Snapshot.Capability, request, account)
	canSwitch := err == nil && result.Rejected && result.Failover != "" && !result.Accepted && result.ID == "" && len(result.Artifacts) == 0
	var scope RejectionScope
	if canSwitch {
		scope = result.Failover
		if result.RetryAfter <= 0 {
			result.RetryAfter = 30 * time.Second
		}
	}
	e.Routes.Finish(ctx, target, owner, scope, result.RetryAfter, result.Accepted)
	attempt := &routing.Attempts[len(routing.Attempts)-1]
	attempt.Outcome = result.Status
	if canSwitch {
		attempt.Outcome = "rejected"
	} else if err != nil {
		attempt.Outcome = "unknown"
	}
	attempt.Status, attempt.Code = result.UpstreamStatusCode, sanitizeMediaDiagnostic(result.UpstreamCode)
	attempt.Reason, attempt.RequestID = sanitizeMediaDiagnostic(result.Diagnostic), sanitizeMediaDiagnostic(result.UpstreamRequestID)
	if canSwitch {
		return e.alternateOrFinish(ctx, task, route, request, result)
	}
	if err == nil && result.Accepted && len(routing.Attempts) > 1 {
		slog.InfoContext(ctx, "media_submission_failover_accepted", "task_id", task.ID, "group_id", task.Identity.GroupID,
			"account_id", task.AccountID, "attempts", len(routing.Attempts))
	}
	return e.completeProcessing(ctx, task, account, result, err)
}

func (e *Engine) alternateOrFinish(ctx context.Context, task StoredTask, current SubmissionRoute, request Request, result UpstreamResult) error {
	routing := task.Snapshot.Routing
	var retry time.Duration
	if len(routing.Attempts) < MaxSubmissionAttempts {
		caps, _, err := e.configuration(ctx, task.Identity.GroupID)
		if err != nil {
			return err
		}
		routes := slices.Clone(routing.Candidates)
		slices.SortStableFunc(routes, func(a, b SubmissionRoute) int {
			return routeOriginRank(a, current) - routeOriginRank(b, current)
		})
		for _, route := range routes {
			if route.AccountID == current.AccountID || slices.ContainsFunc(routing.Attempts, func(attempt SubmissionAttempt) bool {
				return attempt.AccountID == route.AccountID
			}) {
				continue
			}
			if delay := e.Routes.RetryIn(ctx, route.target(task.Identity.GroupID, request.Operation)); delay > 0 {
				retry = earlierRetry(retry, delay)
				continue
			}
			account, lookupErr := e.Lookup(route.AccountID, task.Identity.GroupID)
			if lookupErr != nil || !account.Allowed || route.Origin != RouteOrigin(account.BaseURL, account.Platform) ||
				!slices.ContainsFunc(caps, func(cap Capability) bool {
					return cap.ID == route.Capability.ID && cap.Version == route.Capability.Version && cap.Matches(route.Capability.Model, request.Operation, task.Identity.GroupID)
				}) {
				continue
			}
			prepared, prepareErr := e.prepareSubmissionRoute(ctx, request, current, route)
			if prepareErr != nil {
				continue
			}
			path, writeErr := e.Blobs.WriteJSON(task.ID, fmt.Sprintf("request-%d-%d", task.Epoch, route.AccountID), prepared)
			if writeErr != nil {
				return writeErr
			}
			err = e.Store.requeueOnRoute(ctx, task, route, path, result)
			if mediaAdmissionFailure(err) {
				_ = os.Remove(path)
				retry = earlierRetry(retry, time.Second)
				continue
			}
			if err == nil {
				slog.InfoContext(ctx, "media_submission_switched", "task_id", task.ID, "group_id", task.Identity.GroupID,
					"from_account_id", task.AccountID, "to_account_id", route.AccountID, "attempts", len(routing.Attempts), "upstream_status", result.UpstreamStatusCode)
			}
			return err
		}
	}
	if retry > 0 {
		result.RetryAfter = earlierRetry(result.RetryAfter, retry)
	}
	slog.InfoContext(ctx, "media_submission_routes_exhausted", "task_id", task.ID, "group_id", task.Identity.GroupID,
		"account_id", task.AccountID, "attempts", len(routing.Attempts), "upstream_status", result.UpstreamStatusCode)
	return e.completeProcessing(ctx, task, Account{}, result, nil)
}

func routeOriginRank(route, current SubmissionRoute) int {
	if route.Origin == current.Origin {
		return 1
	}
	return 0
}
