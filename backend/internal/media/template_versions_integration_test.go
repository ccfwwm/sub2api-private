//go:build integration

package media_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/stretchr/testify/require"
)

func TestProtocolVersionsPinUpgradeRollbackArchiveAndRestart(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := &media.Store{DB: db}
	snap := fixture(t, db, "20")
	store.RuntimeCapabilities = func(context.Context, int64) ([]media.Capability, error) {
		capability := snap.Capability
		capability.Status, capability.Enabled, capability.Operations = "enabled", true, []string{"video"}
		return []media.Capability{capability}, nil
	}
	require.NoError(t, store.EnsureBuiltinTemplates(ctx))
	require.NoError(t, (&media.Store{DB: db}).EnsureBuiltinTemplates(ctx))
	template := media.BuiltinTemplates()[0]
	template.ID = media.NewTaskID()
	raw, _ := json.Marshal(template)
	first, err := store.SaveProtocolConfig(ctx, "template", template.ID, 0, snap.Identity.UserID, raw, true, nil)
	require.NoError(t, err)
	binding := media.ModelBinding{ID: media.NewTaskID(), GroupID: snap.Identity.GroupID, Model: snap.Capability.Model, AccountID: snap.Capability.AccountIDs[0], Template: &media.TemplateRef{ID: template.ID, Version: first.Version}, UpstreamModel: "explicit-private-model"}
	raw, _ = json.Marshal(binding)
	bound, err := store.SaveProtocolConfig(ctx, "binding", binding.ID, 0, snap.Identity.UserID, raw, true, nil)
	require.NoError(t, err)
	caps, err := store.ResolveTemplates(ctx, []media.Capability{snap.Capability})
	require.NoError(t, err)
	require.Equal(t, "explicit-private-model", caps[0].UpstreamModel)
	frozen := caps[0]
	template.Rules = []media.ParameterRule{{Phase: "semantic", Op: "default", Target: "seconds", Value: 8}}
	raw, _ = json.Marshal(template)
	next, err := store.SaveProtocolConfig(ctx, "template", template.ID, first.Version, snap.Identity.UserID, raw, true, nil)
	require.NoError(t, err)
	caps, err = store.ResolveTemplates(ctx, []media.Capability{snap.Capability})
	require.NoError(t, err)
	require.Equal(t, first.Version, caps[0].TemplateRef.Version)
	third, err := store.SaveProtocolConfig(ctx, "template", template.ID, next.Version, snap.Identity.UserID, raw, true, []string{binding.ID})
	require.NoError(t, err)
	restarted := &media.Store{DB: db}
	caps, err = restarted.ResolveTemplates(ctx, []media.Capability{snap.Capability})
	require.NoError(t, err)
	require.Equal(t, third.Version, caps[0].TemplateRef.Version)
	require.NotEqual(t, frozen.Version, caps[0].Version)
	require.Equal(t, first.Version, frozen.TemplateRef.Version)
	require.Empty(t, frozen.Template.Rules)
	history, err := restarted.History(ctx, "binding", binding.ID)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, bound.Version+1, history[0].Version)
	original, err := store.ConfigVersion(ctx, "template", template.ID, first.Version)
	require.NoError(t, err)
	rolled, err := store.SaveProtocolConfig(ctx, "template", template.ID, third.Version, snap.Identity.UserID, original.Config, true, []string{binding.ID})
	require.NoError(t, err)
	_, err = store.ArchiveProtocol(ctx, "template", template.ID, rolled.Version, snap.Identity.UserID)
	require.NoError(t, err)
	caps, err = restarted.ResolveTemplates(ctx, []media.Capability{snap.Capability})
	require.NoError(t, err)
	require.NotNil(t, caps[0].Template)
	require.Equal(t, rolled.Version, caps[0].TemplateRef.Version)
	require.Empty(t, caps[0].Template.Rules)
	// A failed selected upgrade rolls back the template publication itself.
	records, err := store.History(ctx, "template", template.ID)
	require.NoError(t, err)
	before := len(records)
	_, err = store.SaveProtocolConfig(ctx, "template", template.ID, rolled.Version+1, snap.Identity.UserID, original.Config, true, []string{"missing-binding"})
	require.Error(t, err)
	records, err = store.History(ctx, "template", template.ID)
	require.NoError(t, err)
	require.Len(t, records, before)
}

func TestProtocolConcurrentSelectorsCannotPublishTwice(t *testing.T) {
	db := testDB(t)
	store := &media.Store{DB: db}
	snap := fixture(t, db, "20")
	store.RuntimeCapabilities = func(context.Context, int64) ([]media.Capability, error) {
		capability := snap.Capability
		capability.Status, capability.Enabled, capability.Operations = "enabled", true, []string{"video"}
		return []media.Capability{capability}, nil
	}
	ctx := context.Background()
	require.NoError(t, store.EnsureBuiltinTemplates(ctx))
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := media.ModelBinding{ID: media.NewTaskID(), GroupID: snap.Identity.GroupID, Model: snap.Capability.Model, Template: &media.TemplateRef{ID: "video-arrays", Version: 1}}
			raw, _ := json.Marshal(b)
			_, err := store.SaveProtocolConfig(ctx, "binding", b.ID, 0, snap.Identity.UserID, raw, true, nil)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorContains(t, err, "binding_conflict")
		}
	}
	require.Equal(t, 1, success)
}

func TestTemplateUpgradeRollsBackInheritedRuleConflict(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	snap := fixture(t, db, "20")
	store := &media.Store{DB: db, RuntimeCapabilities: func(context.Context, int64) ([]media.Capability, error) {
		capability := snap.Capability
		capability.Status, capability.Enabled, capability.Operations = "enabled", true, []string{"video"}
		return []media.Capability{capability}, nil
	}}
	template := media.BuiltinTemplates()[0]
	template.ID = media.NewTaskID()
	raw, err := json.Marshal(template)
	require.NoError(t, err)
	first, err := store.SaveProtocolConfig(ctx, "template", template.ID, 0, snap.Identity.UserID, raw, true, nil)
	require.NoError(t, err)
	binding := media.ModelBinding{ID: media.NewTaskID(), GroupID: snap.Identity.GroupID, Model: snap.Capability.Model, AccountID: snap.Capability.AccountIDs[0], Template: &media.TemplateRef{ID: template.ID, Version: first.Version}}
	raw, err = json.Marshal(binding)
	require.NoError(t, err)
	_, err = store.SaveProtocolConfig(ctx, "binding", binding.ID, 0, snap.Identity.UserID, raw, true, nil)
	require.NoError(t, err)
	group := media.ModelBinding{ID: media.NewTaskID(), GroupID: snap.Identity.GroupID, Rules: []media.ParameterRule{{Phase: "wire", Op: "fixed", Target: "request.flag", Value: true}}}
	raw, err = json.Marshal(group)
	require.NoError(t, err)
	_, err = store.SaveProtocolConfig(ctx, "binding", group.ID, 0, snap.Identity.UserID, raw, true, nil)
	require.NoError(t, err)

	template.Rules = []media.ParameterRule{{Phase: "wire", Op: "fixed", Target: "request", Value: map[string]any{}}}
	raw, err = json.Marshal(template)
	require.NoError(t, err)
	_, err = store.SaveProtocolConfig(ctx, "template", template.ID, first.Version, snap.Identity.UserID, raw, true, []string{binding.ID})
	require.ErrorContains(t, err, "parameter_rule_conflict")
	templateHistory, err := store.History(ctx, "template", template.ID)
	require.NoError(t, err)
	require.Len(t, templateHistory, 1)
	bindingHistory, err := store.History(ctx, "binding", binding.ID)
	require.NoError(t, err)
	require.Len(t, bindingHistory, 1)
	var original media.ModelBinding
	require.NoError(t, json.Unmarshal(bindingHistory[0].Config, &original))
	require.Equal(t, first.Version, original.Template.Version)
}
