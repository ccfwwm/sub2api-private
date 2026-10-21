package media

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type TemplateResolutionIssue struct {
	GroupID    int64
	AccountID  int64
	Model      string
	BindingIDs []string
	Err        error
}

//go:embed templates.json
var builtinTemplateJSON []byte

func BuiltinTemplates() []ProtocolTemplate {
	var out []ProtocolTemplate
	_ = json.Unmarshal(builtinTemplateJSON, &out)
	return out
}

func (s *Store) EnsureBuiltinTemplates(ctx context.Context) error {
	s.templateSeed.Do(func() { s.templateSeedError = s.seedBuiltinTemplates(ctx) })
	return s.templateSeedError
}
func (s *Store) seedBuiltinTemplates(ctx context.Context) error {
	// Never replace a head or administrator edits when a shipped template changes.
	for _, t := range BuiltinTemplates() {
		if err := ValidateTemplate(t, t.Status == "enabled"); err != nil {
			return fmt.Errorf("builtin %s: %w", t.ID, err)
		}
		raw, _ := json.Marshal(t)
		_, err := s.DB.ExecContext(ctx, `WITH seed AS (
 INSERT INTO media_config_versions(kind,id,version,status,config,actor_id)
 VALUES('template',$1,1,$2,$3,0) ON CONFLICT DO NOTHING RETURNING id
) INSERT INTO media_config_heads(kind,id,version,published_version)
 SELECT 'template',id,1,CASE WHEN $2='enabled' THEN 1 ELSE NULL END FROM seed ON CONFLICT DO NOTHING`, t.ID, t.Status, raw)
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ArchiveProtocol(ctx context.Context, kind, id string, expected, actor int64) (ConfigRecord, error) {
	if kind != "template" && kind != "binding" {
		return ConfigRecord{}, fail(400, "invalid_config_kind", "Protocol configuration required")
	}
	return s.DisableConfig(ctx, kind, id, expected, actor)
}
func (s *Store) ConfigVersion(ctx context.Context, kind, id string, version int64) (ConfigRecord, error) {
	var r ConfigRecord
	err := s.DB.QueryRowContext(ctx, `SELECT id,version,NULL::bigint,status,config FROM media_config_versions WHERE kind=$1 AND id=$2 AND version=$3`, kind, id, version).Scan(&r.ID, &r.Version, &r.PublishedVersion, &r.Status, &r.Config)
	if err == sql.ErrNoRows {
		return r, fail(422, "configuration_version_missing", "Pinned configuration version does not exist")
	}
	return r, err
}
func (s *Store) validateBindingPublication(ctx context.Context, tx *sql.Tx, b ModelBinding) error {
	templates := map[TemplateRef]json.RawMessage{}
	load := func(ref TemplateRef) (ProtocolTemplate, error) {
		raw, ok := templates[ref]
		if !ok {
			var status string
			if err := tx.QueryRowContext(ctx, `SELECT status,config FROM media_config_versions WHERE kind='template' AND id=$1 AND version=$2`, ref.ID, ref.Version).Scan(&status, &raw); err != nil || status != "enabled" {
				return ProtocolTemplate{}, fail(422, "template_version_not_published", "Bind a published template version")
			}
			templates[ref] = raw
		}
		var template ProtocolTemplate
		if err := json.Unmarshal(raw, &template); err != nil {
			return template, err
		}
		return template, nil
	}
	var selectedTemplate *ProtocolTemplate
	if b.Template != nil {
		template, err := load(*b.Template)
		if err != nil {
			return err
		}
		selectedTemplate = &template
	}
	records, err := configRecords(ctx, tx, "capability", true)
	if err != nil {
		return err
	}
	capabilities := make([]Capability, 0, len(records))
	for _, record := range records {
		var capability Capability
		if err := json.Unmarshal(record.Config, &capability); err != nil {
			return err
		}
		capabilities = append(capabilities, capability)
	}
	if s.RuntimeCapabilities != nil {
		runtimeCapabilities, err := s.RuntimeCapabilities(ctx, b.GroupID)
		if err != nil {
			return err
		}
		capabilities = append(capabilities, runtimeCapabilities...)
	}
	if selectedTemplate != nil {
		if err := validateTemplateBindingCompatibility(*selectedTemplate, b, capabilities); err != nil {
			return err
		}
	}
	records, err = configRecords(ctx, tx, "binding", true)
	if err != nil {
		return err
	}
	bindings := make([]ModelBinding, 0, len(records)+1)
	for _, r := range records {
		var other ModelBinding
		if err := json.Unmarshal(r.Config, &other); err != nil {
			return err
		}
		if other.ID != b.ID && other.Status == "enabled" && other.GroupID == b.GroupID && other.Model == b.Model && other.AccountID == b.AccountID {
			return fail(409, "binding_conflict", "A published binding already occupies this selector")
		}
		if other.ID != b.ID {
			bindings = append(bindings, other)
		}
	}
	bindings = append(bindings, b)
	selected := []Capability{}
	for _, capability := range capabilities {
		if capability.Status != "enabled" || !capability.Enabled || !containsInt64(capability.GroupIDs, b.GroupID) || b.Model != "" && capability.Model != b.Model || b.AccountID > 0 && !containsInt64(capability.AccountIDs, b.AccountID) {
			continue
		}
		// Validate only selectors affected by this publication. Other groups and
		// accounts on a shared capability retain their current configuration.
		capability.GroupIDs = []int64{b.GroupID}
		if b.AccountID > 0 {
			capability.AccountIDs = []int64{b.AccountID}
		}
		selected = append(selected, capability)
	}
	if len(selected) == 0 {
		return fail(409, "binding_capability_missing", "No published capability matches the selected binding")
	}
	resolved, err := ResolveTemplateBindings(selected, bindings, load)
	if err != nil {
		return err
	}
	for _, capability := range resolved {
		if capability.Template == nil {
			return fail(409, "binding_template_missing", "Selected binding does not inherit a published protocol template")
		}
		if !capability.Enabled {
			return fail(409, "binding_template_operation_conflict", "Effective template does not support every operation selected by this binding")
		}
	}
	return nil
}
func (s *Store) upgradeTemplateBindings(ctx context.Context, tx *sql.Tx, id string, version, actor int64, selected []string) error {
	var templateRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT config FROM media_config_versions WHERE kind='template' AND id=$1 AND version=$2 AND status='enabled'`, id, version).Scan(&templateRaw); err != nil {
		return err
	}
	var template ProtocolTemplate
	if err := json.Unmarshal(templateRaw, &template); err != nil {
		return err
	}
	capabilityRecords, err := configRecords(ctx, tx, "capability", true)
	if err != nil {
		return err
	}
	capabilities := make([]Capability, 0, len(capabilityRecords))
	for _, record := range capabilityRecords {
		var capability Capability
		if err := json.Unmarshal(record.Config, &capability); err != nil {
			return err
		}
		capabilities = append(capabilities, capability)
	}

	seen := map[string]bool{}
	for _, bindingID := range selected {
		if seen[bindingID] {
			return fail(409, "duplicate_binding_upgrade", "Select each binding once")
		}
		seen[bindingID] = true
		var current, published int64
		var raw []byte
		err := tx.QueryRowContext(ctx, `SELECT h.version,h.published_version,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND v.version=h.published_version WHERE h.kind='binding' AND h.id=$1 FOR UPDATE OF h`, bindingID).Scan(&current, &published, &raw)
		if err != nil {
			return err
		}
		if current != published {
			return fail(409, "binding_draft_conflict", "Selected binding has an unpublished draft; publish or retain it before upgrading")
		}
		var b ModelBinding
		if err = json.Unmarshal(raw, &b); err != nil {
			return err
		}
		if b.Template == nil || b.Template.ID != id || b.Status != "enabled" {
			return fail(409, "binding_upgrade_conflict", "Selected binding does not use this template")
		}
		if err = s.validateTemplateBindingCompatibility(ctx, template, b, capabilities); err != nil {
			return err
		}
		b.Template.Version = version
		b.Version = strconv.FormatInt(current+1, 10)
		if err = s.validateBindingPublication(ctx, tx, b); err != nil {
			return err
		}
		raw, err = json.Marshal(b)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES('binding',$1,$2,'enabled',$3,$4)`, bindingID, current+1, raw, actor); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE media_config_heads SET version=$1,published_version=$1 WHERE kind='binding' AND id=$2`, current+1, bindingID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) validateTemplateBindingCompatibility(ctx context.Context, template ProtocolTemplate, binding ModelBinding, persisted []Capability) error {
	capabilities := append([]Capability(nil), persisted...)
	if s.RuntimeCapabilities != nil {
		runtimeCapabilities, err := s.RuntimeCapabilities(ctx, binding.GroupID)
		if err != nil {
			return err
		}
		capabilities = append(capabilities, runtimeCapabilities...)
	}
	return validateTemplateBindingCompatibility(template, binding, capabilities)
}

func validateTemplateBindingCompatibility(template ProtocolTemplate, binding ModelBinding, capabilities []Capability) error {
	matched := 0
	for _, capability := range capabilities {
		if capability.Status != "enabled" || !capability.Enabled || !containsInt64(capability.GroupIDs, binding.GroupID) {
			continue
		}
		if binding.Model != "" && capability.Model != binding.Model {
			continue
		}
		if binding.AccountID > 0 && !containsInt64(capability.AccountIDs, binding.AccountID) {
			continue
		}
		matched++
		if !contains(template.MediaTypes, capability.MediaType) {
			return fail(409, "binding_template_media_conflict", "New template does not support every media type selected by this binding")
		}
		for _, operation := range capability.Operations {
			if !contains(template.Operations, Operation(operation)) {
				return fail(409, "binding_template_operation_conflict", "New template does not support every operation selected by this binding")
			}
		}
	}
	if matched == 0 {
		return fail(409, "binding_capability_missing", "No published capability matches the selected binding")
	}
	return nil
}

func ValidateBinding(b ModelBinding, publish bool) error {
	if b.ID == "" || b.GroupID <= 0 || b.AccountID < 0 {
		return fail(400, "invalid_binding", "ID and group required")
	}
	if b.UpstreamModel != "" && b.Model == "" {
		return fail(400, "invalid_model_mapping", "Upstream mapping requires an exact public model")
	}
	if b.AccountID > 0 && b.Model == "" {
		return fail(400, "invalid_binding", "Account overrides require an exact public model")
	}
	if b.Template != nil && (b.Template.ID == "" || b.Template.Version < 1) {
		return fail(400, "invalid_template_reference", "Template versions must be pinned")
	}
	return ValidateParameterRules(b.Rules)
}

func bindingRank(b ModelBinding) int {
	if b.AccountID > 0 {
		return 3
	}
	if b.Model != "" {
		return 2
	}
	return 1
}

func mergeRuleLayers(rules []ParameterRule) []ParameterRule {
	result := []ParameterRule{}
	positions := map[string]int{}
	for _, r := range rules {
		target := r.Target
		if target == "" {
			target = r.Source
		}
		if r.Op == "allow" {
			target = "*"
		}
		key := r.Phase + ":" + target
		if index, ok := positions[key]; ok {
			result[index] = r
		} else {
			positions[key] = len(result)
			result = append(result, r)
		}
	}
	return result
}
func ResolveTemplateBindings(caps []Capability, bindings []ModelBinding, load func(TemplateRef) (ProtocolTemplate, error)) ([]Capability, error) {
	ordered := append([]ModelBinding{}, bindings...)
	sort.SliceStable(ordered, func(i, j int) bool { return bindingRank(ordered[i]) < bindingRank(ordered[j]) })
	expanded := expandCapabilitySelectors(caps)
	out := make([]Capability, 0, len(expanded))
	for _, original := range expanded {
		cap := original
		ref := cap.TemplateRef
		cap.TemplateRules = append([]ParameterRule{}, cap.TemplateRules...)
		// Preserve explicit legacy field overrides. Protocol selection itself is
		// pinned by a persisted template_ref or an administrator binding.
		// Rule order contributes to the public profile and estimate version.
		// Go map iteration must not invalidate an unchanged configuration.
		mappingSources := make([]string, 0, len(cap.FieldMapping))
		for source := range cap.FieldMapping {
			mappingSources = append(mappingSources, source)
		}
		sort.Strings(mappingSources)
		for _, source := range mappingSources {
			target := cap.FieldMapping[source]
			if source != target && source != "model" && !contains([]string{"image", "video", "audio", "mask", "images", "videos", "audios", "masks"}, source) && target != "" && configPath.MatchString(target) && !strings.ContainsAny(target, ":[") {
				cap.TemplateRules = append(cap.TemplateRules, ParameterRule{Phase: "wire", Op: "rename", Source: source, Target: target})
			}
		}
		seen := map[int]bool{}
		for _, b := range ordered {
			if b.Status != "enabled" || !containsInt64(cap.GroupIDs, b.GroupID) || b.Model != "" && b.Model != cap.Model || b.AccountID > 0 && !containsInt64(cap.AccountIDs, b.AccountID) {
				continue
			}
			rank := bindingRank(b)
			if seen[rank] {
				return nil, fail(409, "binding_conflict", "Overlapping published bindings at the same priority")
			}
			seen[rank] = true
			if b.Template != nil {
				ref = b.Template
			}
			if b.UpstreamModel != "" {
				cap.UpstreamModel = b.UpstreamModel
			}
			cap.TemplateRules = append(cap.TemplateRules, b.Rules...)
			version, _ := strconv.ParseInt(b.Version, 10, 64)
			cap.BindingVersions = append(cap.BindingVersions, TemplateRef{ID: b.ID, Version: version})
		}
		if ref == nil {
			cap.Enabled = false
			cap.ValidationStatus = "待配置协议模板"
			out = append(out, cap)
			continue
		}
		t, err := load(*ref)
		if err != nil {
			return nil, err
		}
		for i, m := range t.Materials {
			target := original.FieldMapping[m.Kind+"s"]
			if target == "" {
				target = original.FieldMapping[m.Kind]
				// Legacy single-material paths include the serialized URL leaf.
				// The explicit template already supplies that envelope; applying a
				// rename would reject image -> image.url or wrap the URL twice.
				if m.Shape == "url" && strings.HasSuffix(target, ".url") {
					target = strings.TrimSuffix(target, ".url")
				}
			}
			if target != "" && configPath.MatchString(target) && !strings.ContainsAny(target, ":[") {
				t.Materials[i].Target = target
			}
		}
		for target, path := range original.UsageFields {
			if t.Response.Usage == nil {
				t.Response.Usage = map[string]string{}
			}
			t.Response.Usage[target] = path
		}
		cap.TemplateRules = mergeRuleLayers(cap.TemplateRules)
		if err := ValidateParameterRules(mergeRuleLayers(append(append([]ParameterRule{}, t.Rules...), cap.TemplateRules...))); err != nil {
			return nil, err
		}
		if t.Status != "enabled" {
			cap.Enabled = false
			cap.ValidationStatus = "模板版本未发布"
		}
		if !contains(t.MediaTypes, cap.MediaType) {
			return nil, fail(409, "template_media_conflict", "Template does not support the configured media type")
		}
		for _, op := range cap.Operations {
			if !contains(t.Operations, op) {
				cap.Enabled = false
				cap.ValidationStatus = "模板操作待配置"
			}
		}
		cap.Template = &t
		cap.TemplateRef = ref
		cap.Adapter = "template"
		cap.RequestFormat = "unified"
		cap.Delivery.PollSeconds = t.PollSeconds
		cap.Version = fmt.Sprintf("%s:t%s:%d:%s", original.Version, ref.ID, ref.Version, configurationDigest(cap.TemplateRules, cap.BindingVersions, cap.UpstreamModel))
		out = append(out, cap)
	}
	return out, nil
}

func expandCapabilitySelectors(caps []Capability) []Capability {
	expanded := []Capability{}
	for _, cap := range caps {
		if len(cap.GroupIDs) > 1 || len(cap.AccountIDs) > 1 {
			for _, group := range cap.GroupIDs {
				for _, account := range cap.AccountIDs {
					candidate := cap
					candidate.GroupIDs = []int64{group}
					candidate.AccountIDs = []int64{account}
					candidate.ID = fmt.Sprintf("%s:g%d:a%d", cap.ID, group, account)
					expanded = append(expanded, candidate)
				}
			}
		} else {
			expanded = append(expanded, cap)
		}
	}
	return expanded
}

// ResolveTemplateBindingsIsolated retains disabled selectors so failed profiles
// cannot fall back to legacy routing. Infrastructure errors still fail the read.
func ResolveTemplateBindingsIsolated(caps []Capability, bindings []ModelBinding, load func(TemplateRef) (ProtocolTemplate, error)) ([]Capability, []TemplateResolutionIssue, error) {
	expanded := expandCapabilitySelectors(caps)
	resolved := make([]Capability, 0, len(expanded))
	issues := []TemplateResolutionIssue{}
	for _, candidate := range expanded {
		items, err := ResolveTemplateBindings([]Capability{candidate}, bindings, load)
		if err == nil {
			resolved = append(resolved, items...)
			continue
		}
		var configError *Error
		if !errors.As(err, &configError) || configError.Status >= 500 {
			return nil, issues, err
		}

		candidate.Enabled = false
		candidate.ValidationStatus = "协议绑定配置错误: " + err.Error()
		resolved = append(resolved, candidate)
		issue := TemplateResolutionIssue{Model: candidate.Model, Err: err}
		if len(candidate.GroupIDs) > 0 {
			issue.GroupID = candidate.GroupIDs[0]
		}
		if len(candidate.AccountIDs) > 0 {
			issue.AccountID = candidate.AccountIDs[0]
		}
		for _, binding := range bindings {
			if binding.Status == "enabled" && containsInt64(candidate.GroupIDs, binding.GroupID) && (binding.Model == "" || binding.Model == candidate.Model) && (binding.AccountID == 0 || containsInt64(candidate.AccountIDs, binding.AccountID)) {
				issue.BindingIDs = append(issue.BindingIDs, binding.ID)
			}
		}
		issues = append(issues, issue)
	}
	return resolved, issues, nil
}
func configurationDigest(values ...any) string {
	raw, _ := json.Marshal(values)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func containsInt64(values []int64, id int64) bool {
	for _, v := range values {
		if v == id {
			return true
		}
	}
	return false
}
func (s *Store) ResolveTemplates(ctx context.Context, caps []Capability) ([]Capability, error) {
	if err := s.EnsureBuiltinTemplates(ctx); err != nil {
		return nil, err
	}
	records, err := s.Configs(ctx, "binding", true)
	if err != nil {
		return nil, err
	}
	bindings := []ModelBinding{}
	for _, r := range records {
		var b ModelBinding
		if err = json.Unmarshal(r.Config, &b); err != nil {
			return nil, err
		}
		bindings = append(bindings, b)
	}
	return ResolveTemplateBindings(caps, bindings, func(ref TemplateRef) (ProtocolTemplate, error) {
		var t ProtocolTemplate
		r, err := s.ConfigVersion(ctx, "template", ref.ID, ref.Version)
		if err != nil {
			return t, err
		}
		err = json.Unmarshal(r.Config, &t)
		return t, err
	})
}

func (s *Store) ResolveTemplatesIsolated(ctx context.Context, caps []Capability) ([]Capability, []TemplateResolutionIssue, error) {
	if err := s.EnsureBuiltinTemplates(ctx); err != nil {
		return nil, nil, err
	}
	records, err := s.Configs(ctx, "binding", true)
	if err != nil {
		return nil, nil, err
	}
	bindings := make([]ModelBinding, 0, len(records))
	for _, record := range records {
		var binding ModelBinding
		if err := json.Unmarshal(record.Config, &binding); err != nil {
			// A malformed selector cannot be safely scoped to an account/model.
			return nil, nil, fmt.Errorf("binding %s: %w", record.ID, err)
		}
		bindings = append(bindings, binding)
	}
	cache := map[TemplateRef]json.RawMessage{}
	load := func(ref TemplateRef) (ProtocolTemplate, error) {
		raw, ok := cache[ref]
		if !ok {
			record, err := s.ConfigVersion(ctx, "template", ref.ID, ref.Version)
			if err != nil {
				return ProtocolTemplate{}, err
			}
			raw = record.Config
			cache[ref] = raw
		}
		var template ProtocolTemplate
		if err := json.Unmarshal(raw, &template); err != nil {
			return ProtocolTemplate{}, fail(422, "invalid_template_config", "Pinned template has invalid fields")
		}
		return template, nil
	}
	return ResolveTemplateBindingsIsolated(caps, bindings, load)
}
