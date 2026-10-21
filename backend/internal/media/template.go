package media

// Protocol templates are declarative and contain no credentials or executable
// expressions. A task retains the complete resolved version, never a live head.
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type TemplateRef struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type TemplateAuth struct {
	Mode   string `json:"mode"` // bearer, header, query, none; value is always the account secret
	Name   string `json:"name,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}
type TemplateEndpoint struct {
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Query    map[string]string `json:"query,omitempty"`
	Encoding string            `json:"encoding,omitempty"` // json, multipart, form
}
type ParameterRule struct {
	Phase      string         `json:"phase"` // semantic before pricing; wire after pricing
	Op         string         `json:"op"`
	Source     string         `json:"source,omitempty"`
	Target     string         `json:"target,omitempty"`
	Value      any            `json:"value,omitempty"`
	Values     map[string]any `json:"values,omitempty"`
	OnUnmapped string         `json:"on_unmapped,omitempty"` // error (default), preserve
	Fields     []string       `json:"fields,omitempty"`
	Type       string         `json:"type,omitempty"`
}
type MaterialRule struct {
	Kind     string         `json:"kind"`
	Target   string         `json:"target"`
	Encoding string         `json:"encoding"` // preserve, data_url, base64
	Shape    string         `json:"shape"`    // preserve, string, url, b64_json, data_mime, content, parts
	Single   bool           `json:"single,omitempty"`
	MaxCount int            `json:"max_count,omitempty"`
	Role     string         `json:"role,omitempty"`
	Wrapper  map[string]any `json:"wrapper,omitempty"`
}
type TemplateResponse struct {
	PCMToWAV *PCMToWAVConfig      `json:"pcm_to_wav,omitempty"`
	TaskID   []string             `json:"task_id,omitempty"`
	Status   []string             `json:"status,omitempty"`
	Progress []string             `json:"progress,omitempty"`
	Errors   []string             `json:"errors,omitempty"`
	States   map[string]string    `json:"states,omitempty"`
	URLs     []string             `json:"urls,omitempty"`
	Base64   []string             `json:"base64,omitempty"`
	MIME     string               `json:"mime,omitempty"`
	Usage    map[string]string    `json:"usage,omitempty"`
	Links    []string             `json:"links,omitempty"`
	Results  []TemplateResultRule `json:"results,omitempty"`
}
type TemplateResultRule struct {
	Path        string `json:"path"`
	URL         string `json:"url,omitempty"`
	Base64      string `json:"base64,omitempty"`
	MIME        string `json:"mime,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ArtifactID  string `json:"artifact_id,omitempty"`
	ContentPath string `json:"content_path,omitempty"`
}
type ProtocolTemplate struct {
	ID              string                      `json:"id"`
	Version         string                      `json:"version"`
	Status          string                      `json:"status"`
	SchemaVersion   int                         `json:"schema_version"`
	Name            string                      `json:"name"`
	Description     string                      `json:"description,omitempty"`
	MediaTypes      []string                    `json:"media_types"`
	Operations      []string                    `json:"operations"`
	Mode            string                      `json:"mode"` // sync_json, sync_binary, async
	Auth            TemplateAuth                `json:"auth"`
	Discovery       *TemplateEndpoint           `json:"discovery,omitempty"`
	ModelIDs        []string                    `json:"model_ids,omitempty"`
	Submit          TemplateEndpoint            `json:"submit"`
	OperationSubmit map[string]TemplateEndpoint `json:"operation_submit,omitempty"`
	Query           *TemplateEndpoint           `json:"query,omitempty"`
	Recover         *TemplateEndpoint           `json:"recover,omitempty"`
	ArtifactList    *TemplateEndpoint           `json:"artifact_list,omitempty"`
	ContentPath     string                      `json:"content_path,omitempty"`
	DownloadAuth    *TemplateAuth               `json:"download_auth,omitempty"`
	Response        TemplateResponse            `json:"response"`
	PollSeconds     int                         `json:"poll_seconds"`
	Rules           []ParameterRule             `json:"rules,omitempty"`
	Materials       []MaterialRule              `json:"materials,omitempty"`
	ModelTarget     string                      `json:"model_target,omitempty"`
	Body            map[string]any              `json:"body,omitempty"`
	BuiltinRevision string                      `json:"builtin_revision,omitempty"`
}
type ModelBinding struct {
	ID            string          `json:"id"`
	Version       string          `json:"version"`
	Status        string          `json:"status"`
	GroupID       int64           `json:"group_id"`
	Model         string          `json:"model,omitempty"`
	AccountID     int64           `json:"account_id,omitempty"`
	UpstreamModel string          `json:"upstream_model,omitempty"`
	Template      *TemplateRef    `json:"template,omitempty"`
	Rules         []ParameterRule `json:"rules,omitempty"`
}
type PreparedMediaRequest struct {
	Method   string         `json:"method"`
	Path     string         `json:"path"`
	Encoding string         `json:"encoding"`
	Body     map[string]any `json:"body"`
	Files    []Upload       `json:"files,omitempty"`
}

var configPath = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\[[0-9]*\])?(\.[A-Za-z_][A-Za-z0-9_]*(\[[0-9]*\])?)*$`)
var pathVariable = regexp.MustCompile(`\{([a-z_]+)\}`)

func validTemplatePath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\\r\n?#") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	for _, match := range pathVariable.FindAllStringSubmatch(path, -1) {
		if !contains([]string{"task_id", "model", "artifact_id", "idempotency_key"}, match[1]) {
			return false
		}
	}
	return !strings.ContainsAny(pathVariable.ReplaceAllString(path, ""), "{}")
}
func ValidateTemplate(t ProtocolTemplate, publish bool) error {
	serialized, _ := json.Marshal(t)
	if inlineReviewCredential.Match(serialized) {
		return fail(400, "template_secret_forbidden", "Templates reference account authentication; embedded credentials are forbidden")
	}
	if t.ID == "" || t.Name == "" || t.SchemaVersion != 1 {
		return fail(400, "invalid_template", "ID, name and schema_version 1 required")
	}
	if !contains([]string{"sync_json", "sync_binary", "async"}, t.Mode) {
		return fail(400, "invalid_template_mode", "Unknown execution mode")
	}
	if t.PollSeconds < 1 || t.PollSeconds > 300 {
		return fail(400, "invalid_poll_interval", "Poll interval must be 1 to 300 seconds")
	}
	for _, auth := range []*TemplateAuth{&t.Auth, t.DownloadAuth} {
		if auth == nil {
			continue
		}
		if !contains([]string{"bearer", "header", "query", "none"}, auth.Mode) {
			return fail(400, "invalid_auth", "Unknown account credential reference")
		}
		if (auth.Mode == "header" || auth.Mode == "query") && !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`).MatchString(auth.Name) {
			return fail(400, "invalid_auth", "Authentication name required")
		}
		if strings.ContainsAny(auth.Prefix, "\r\n") {
			return fail(400, "invalid_auth", "Invalid authentication prefix")
		}
	}
	for _, ep := range []*TemplateEndpoint{&t.Submit, t.Query, t.Discovery, t.Recover, t.ArtifactList} {
		if ep == nil {
			continue
		}
		if !validTemplatePath(ep.Path) || !contains([]string{"GET", "POST", "PUT"}, ep.Method) {
			return fail(400, "invalid_endpoint", "Relative endpoint and explicit HTTP method required")
		}
		if ep.Encoding != "" && !contains([]string{"json", "multipart", "form"}, ep.Encoding) {
			return fail(400, "invalid_encoding", "Unsupported body encoding")
		}
	}
	for op, ep := range t.OperationSubmit {
		if !contains(t.Operations, op) || !validTemplatePath(ep.Path) || !contains([]string{"GET", "POST", "PUT"}, ep.Method) || !contains([]string{"json", "form", "multipart"}, ep.Encoding) {
			return fail(400, "invalid_operation_endpoint", "Invalid explicit operation endpoint")
		}
	}
	if t.ModelTarget != "" && t.ModelTarget != "-" && (!configPath.MatchString(t.ModelTarget) || strings.Contains(t.ModelTarget, "[]")) {
		return fail(400, "invalid_model_target", "Model target must be an object path or - for path-only model routing")
	}
	for _, ep := range []*TemplateEndpoint{t.Discovery, t.Recover, t.ArtifactList, t.Query} {
		if ep != nil && ep.Method != "GET" {
			return fail(400, "readonly_endpoint_required", "Discovery, recovery, polling and artifact listing must be read only")
		}
	}
	if t.ContentPath != "" && !validTemplatePath(t.ContentPath) {
		return fail(400, "invalid_endpoint", "Invalid content path")
	}
	if publish && (len(t.Operations) == 0 || len(t.MediaTypes) == 0 || (t.Mode == "async" && t.Query == nil)) {
		return fail(400, "incomplete_template", "Operations, media types and asynchronous query required")
	}
	for _, op := range t.Operations {
		if !contains([]string{"video", "image", "image_edit", "gemini_image", "audio", "chat_music", "speech"}, op) {
			return fail(400, "invalid_operation", "Unknown operation")
		}
	}
	if err := ValidateParameterRules(t.Rules); err != nil {
		return err
	}
	if err := validatePCMToWAV(t.Response.PCMToWAV); err != nil {
		return err
	}
	if err := validateTemplateBody(t.Body); err != nil {
		return err
	}
	targets := map[string]bool{}
	for _, m := range t.Materials {
		if !contains([]string{"image", "video", "audio", "mask"}, m.Kind) || !configPath.MatchString(m.Target) || strings.Contains(m.Target, "[]") || !contains([]string{"preserve", "data_url", "base64"}, m.Encoding) || !contains([]string{"preserve", "string", "url", "b64_json", "data_mime", "content", "parts"}, m.Shape) || m.MaxCount < 0 {
			return fail(400, "invalid_material_rule", "Invalid material mapping")
		}
		key := m.Kind + ":" + m.Role
		if targets[key] {
			return fail(409, "material_rule_conflict", "Duplicate material selector")
		}
		targets[key] = true
		if targets["all:"+m.Kind] || (m.Role == "" && (targets[m.Kind+":first_frame"] || targets[m.Kind+":last_frame"])) {
			return fail(409, "material_rule_conflict", "Overlapping role selectors")
		}
		if m.Role == "" {
			targets["all:"+m.Kind] = true
		}
	}
	for _, paths := range [][]string{t.Response.TaskID, t.Response.Status, t.Response.Progress, t.Response.Errors, t.Response.URLs, t.Response.Base64, t.Response.Links, t.ModelIDs} {
		for _, p := range paths {
			if !configPath.MatchString(p) {
				return fail(400, "invalid_extract_path", "Only object paths and [] array traversal allowed")
			}
		}
	}
	for target, path := range t.Response.Usage {
		if !contains([]string{"provider_duration", "provider_tokens.total_tokens", "provider_tokens.input_tokens", "provider_tokens.output_tokens", "provider_tokens.image_input_tokens", "provider_tokens.image_output_tokens"}, target) || !configPath.MatchString(path) {
			return fail(400, "invalid_usage_mapping", "Templates can only map provider usage; request and measured quantities remain server owned")
		}
	}
	for _, result := range t.Response.Results {
		for _, p := range []string{result.Path, result.URL, result.Base64, result.MIME, result.Filename, result.ArtifactID} {
			if p != "" && !configPath.MatchString(p) {
				return fail(400, "invalid_result_path", "Invalid result extraction path")
			}
		}
		if result.ContentPath != "" && !validTemplatePath(result.ContentPath) {
			return fail(400, "invalid_endpoint", "Invalid artifact content path")
		}
	}
	for _, state := range t.Response.States {
		if !contains([]string{"failed", "completed", "in_progress", "unknown"}, state) {
			return fail(400, "invalid_state_mapping", "Invalid task state")
		}
	}
	return nil
}

func validateTemplateBody(value any) error {
	switch node := value.(type) {
	case map[string]any:
		for key, item := range node {
			if key == "model" {
				ref, ok := item.(map[string]any)
				if !ok || len(ref) != 1 || (ref["$variable"] != "model" && ref["$input"] != "model") {
					return fail(400, "model_mapping_reserved", "Body model fields must reference the dedicated model mapping")
				}
			}
			if err := validateTemplateBody(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range node {
			if err := validateTemplateBody(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func referencedBodyPath(value any, source, path string) string {
	switch node := value.(type) {
	case map[string]any:
		if node["$input"] == source && len(node) == 1 {
			return path
		}
		for key, item := range node {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if found := referencedBodyPath(item, source, next); found != "" {
				return found
			}
		}
	case []any:
		for i, item := range node {
			if found := referencedBodyPath(item, source, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	}
	return ""
}

func verifyBillingWire(t *ProtocolTemplate, req Request, wire map[string]any, rules []ParameterRule) error {
	fields := []string{"n", "resolution", "size", "ratio", "quality"}
	if req.Operation == "video" {
		fields = append(fields, "seconds")
	}
	if req.Operation == "speech" {
		fields = append(fields, "input")
	}
	for _, field := range fields {
		expected, present := req.Parameters[field]
		if !present {
			continue
		}
		path := field
		for _, r := range rules {
			if r.Phase != "wire" || r.Source != path {
				continue
			}
			switch r.Op {
			case "rename":
				path = r.Target
			case "wrap":
				expected = []any{expected}
				if r.Target != "" {
					path = r.Target
				}
			}
		}
		if t.Body != nil {
			path = referencedBodyPath(t.Body, path, "")
		}
		actual, ok := readPath(wire, path)
		if path == "" || !ok || !equivalentBillingValue(expected, actual) {
			return fail(409, "billing_semantics_changed", "Sending rules removed or changed priced field: "+field+"; apply semantic rules before pricing")
		}
	}
	return nil
}

func equivalentBillingValue(a, b any) bool {
	if aa, ok := a.([]any); ok {
		bb, ok := b.([]any)
		if !ok || len(aa) != len(bb) {
			return false
		}
		for i := range aa {
			if !equivalentBillingValue(aa[i], bb[i]) {
				return false
			}
		}
		return true
	}
	return stringValue(a) == stringValue(b)
}
func ValidateParameterRules(rules []ParameterRule) error {
	targets := map[string]bool{}
	moves := map[string]string{}
	for index, r := range rules {
		if !contains([]string{"semantic", "wire"}, r.Phase) || !contains([]string{"default", "fixed", "rename", "copy", "map", "convert", "wrap", "remove", "allow"}, r.Op) {
			return fail(400, "invalid_parameter_rule", "Unknown phase or operation")
		}
		for _, p := range append([]string{r.Source, r.Target}, r.Fields...) {
			if p != "" && (!configPath.MatchString(p) || strings.Contains(p, "[]")) {
				return fail(400, "invalid_parameter_path", "Writable paths must name object fields")
			}
			parts := pathParts(p)
			if contains(parts, "model") {
				return fail(400, "model_mapping_reserved", "Use the dedicated upstream model binding")
			}
		}
		if r.Op != "allow" && r.Target == "" && r.Source == "" {
			return fail(400, "invalid_parameter_rule", "A field is required")
		}
		target := r.Target
		if target == "" {
			target = r.Source
		}
		if r.Op == "allow" {
			target = "*"
		}
		// Source-only rules also write a target. Treat array and dot notation
		// identically, and reject parent/child writes before their order can
		// silently replace an earlier rule's output.
		key := r.Phase + ":" + strings.Join(pathParts(target), ".")
		for previous := range targets {
			if previous == key || strings.HasPrefix(previous, key+".") || strings.HasPrefix(key, previous+".") {
				return fail(409, "parameter_rule_conflict", fmt.Sprintf("Rule %d (%s %s): target %q overlaps %q", index+1, r.Phase, r.Op, target, strings.TrimPrefix(previous, r.Phase+":")))
			}
		}
		targets[key] = true
		if r.OnUnmapped != "" && (r.Op != "map" || !contains([]string{"error", "preserve"}, r.OnUnmapped)) {
			return fail(400, "invalid_mapping_fallback", "on_unmapped must be error or preserve on a map rule")
		}
		if r.Phase == "wire" {
			for _, p := range []string{r.Source, r.Target} {
				parts := pathParts(p)
				if contains([]string{"seconds", "duration", "resolution", "n", "input", "size", "ratio", "aspect_ratio", "quality"}, parts[len(parts)-1]) && !contains([]string{"rename", "copy", "wrap", "convert"}, r.Op) {
					return fail(400, "billing_semantics_rule", "Changes to billing inputs must use the semantic phase")
				}
			}
		}
		if r.Op == "rename" {
			if r.Source == "" || r.Target == "" {
				return fail(400, "invalid_parameter_rule", "Rename requires source and target")
			}
			source := strings.Join(pathParts(r.Source), ".")
			destination := strings.Join(pathParts(r.Target), ".")
			if strings.HasPrefix(source, destination+".") || strings.HasPrefix(destination, source+".") {
				return fail(409, "parameter_rule_conflict", fmt.Sprintf("Rule %d (%s rename): source %q and target %q cannot contain one another", index+1, r.Phase, r.Source, r.Target))
			}
			if source != destination {
				moves[r.Phase+":"+source] = r.Phase + ":" + destination
			}
		}
		if r.Op == "convert" && !contains([]string{"string", "number", "integer", "boolean"}, r.Type) {
			return fail(400, "invalid_conversion", "Unsupported conversion type")
		}
	}
	for start := range moves {
		seen := map[string]bool{}
		for p := start; moves[p] != ""; p = moves[p] {
			if seen[p] {
				return fail(409, "parameter_mapping_cycle", "Cyclic parameter mapping")
			}
			seen[p] = true
		}
	}
	return nil
}

func cloneObject(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
func pathParts(path string) []string {
	return strings.Split(strings.ReplaceAll(strings.ReplaceAll(path, "[", "."), "]", ""), ".")
}
func readPath(in any, path string) (any, bool) {
	v := in
	for _, p := range pathParts(path) {
		switch node := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = node[p]
			if !ok {
				return nil, false
			}
		case []any:
			i, e := strconv.Atoi(p)
			if e != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			v = node[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func writeNode(node any, parts []string, value any, remove bool) (any, error) {
	if len(parts) == 0 {
		return value, nil
	}
	key := parts[0]
	if index, err := strconv.Atoi(key); err == nil {
		if index < 0 || index > 100 {
			return nil, fail(400, "invalid_array_index", "Array index exceeds bounds")
		}
		list, ok := node.([]any)
		if node == nil {
			list = []any{}
			ok = true
		}
		if !ok {
			return nil, fail(409, "parameter_target_conflict", "Target is not an array")
		}
		for len(list) <= index {
			list = append(list, nil)
		}
		if remove && len(parts) == 1 {
			return nil, fail(400, "invalid_array_remove", "Remove an object field instead of an array element")
		}
		next, e := writeNode(list[index], parts[1:], value, remove)
		if e != nil {
			return nil, e
		}
		list[index] = next
		return list, nil
	}
	obj, ok := node.(map[string]any)
	if node == nil {
		obj = map[string]any{}
		ok = true
	}
	if !ok {
		return nil, fail(409, "parameter_target_conflict", "Target is not an object")
	}
	if len(parts) == 1 {
		if remove {
			delete(obj, key)
		} else {
			obj[key] = value
		}
		return obj, nil
	}
	if remove {
		if _, exists := obj[key]; !exists {
			return obj, nil
		}
	}
	next, e := writeNode(obj[key], parts[1:], value, remove)
	if e != nil {
		return nil, e
	}
	obj[key] = next
	return obj, nil
}
func writePath(in map[string]any, path string, v any, remove bool) error {
	_, err := writeNode(in, pathParts(path), v, remove)
	return err
}
func extractValues(in any, path string) []any {
	parts := strings.Split(path, ".")
	nodes := []any{in}
	for _, part := range parts {
		array := strings.HasSuffix(part, "[]")
		key := strings.TrimSuffix(part, "[]")
		next := []any{}
		for _, node := range nodes {
			if obj, ok := node.(map[string]any); ok {
				if v, present := readPath(obj, key); present {
					if array {
						if list, ok := v.([]any); ok {
							next = append(next, list...)
						}
					} else {
						next = append(next, v)
					}
				}
			}
		}
		nodes = next
	}
	return nodes
}
func templateValue(value any, input map[string]any, vars map[string]string) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		if path, ok := v["$input"].(string); ok {
			if len(v) != 1 || !configPath.MatchString(path) {
				return nil, fail(400, "invalid_input_reference", "Invalid input reference")
			}
			result, present := readPath(input, path)
			if !present {
				return nil, fail(400, "missing_template_input", "Required field: "+path)
			}
			return result, nil
		}
		if key, ok := v["$variable"].(string); ok {
			result, present := vars[key]
			if len(v) != 1 || !present {
				return nil, fail(400, "invalid_variable", "Unknown template variable")
			}
			return result, nil
		}
		out := map[string]any{}
		for key, item := range v {
			mapped, err := templateValue(item, input, vars)
			if err != nil {
				return nil, err
			}
			out[key] = mapped
		}
		return out, nil
	case []any:
		out := []any{}
		for _, item := range v {
			mapped, err := templateValue(item, input, vars)
			if err != nil {
				return nil, err
			}
			out = append(out, mapped)
		}
		return out, nil
	default:
		return value, nil
	}
}
func applyParameterRules(input map[string]any, rules []ParameterRule, phase string) (map[string]any, error) {
	p := cloneObject(input)
	for _, r := range rules {
		if r.Phase != phase {
			continue
		}
		target := r.Target
		if target == "" {
			target = r.Source
		}
		value, present := readPath(p, r.Source)
		if r.Source == "" {
			value, present = readPath(p, target)
		}
		switch r.Op {
		case "default":
			if _, exists := readPath(p, target); exists {
				continue
			}
			value = r.Value
		case "fixed":
			value = r.Value
		case "remove":
			if err := writePath(p, target, nil, true); err != nil {
				return nil, err
			}
			continue
		case "allow":
			for key := range p {
				if key != "model" && !contains(r.Fields, key) {
					delete(p, key)
				}
			}
			continue
		default:
			if !present {
				continue
			}
		}
		switch r.Op {
		case "rename", "copy":
			if old, exists := readPath(p, target); exists && r.Source != target && !reflect.DeepEqual(old, value) {
				return nil, fail(409, "parameter_target_conflict", "Existing target: "+target)
			}
		case "map":
			if mapped, ok := r.Values[stringValue(value)]; ok {
				value = mapped
			} else if r.OnUnmapped != "preserve" {
				return nil, fail(400, "parameter_value_unmapped", "No configured value mapping for field: "+target)
			}
		case "convert":
			var err error
			switch r.Type {
			case "string":
				value = stringValue(value)
			case "number":
				value, err = strconv.ParseFloat(stringValue(value), 64)
			case "integer":
				value, err = strconv.ParseInt(stringValue(value), 10, 64)
			case "boolean":
				value, err = strconv.ParseBool(stringValue(value))
			}
			if err != nil {
				return nil, fail(400, "parameter_conversion_failed", "Conversion failed: "+target)
			}
		case "wrap":
			value = []any{value}
		}
		resolved, err := templateValue(value, p, map[string]string{})
		if err != nil {
			return nil, err
		}
		if err = writePath(p, target, resolved, false); err != nil {
			return nil, err
		}
		if r.Op == "rename" && r.Source != target {
			if err = writePath(p, r.Source, nil, true); err != nil {
				return nil, err
			}
		}
	}
	return p, nil
}

func normalizeTemplate(cap Capability, operation string, input map[string]any, uploads []Upload) (Request, error) {
	r := Request{Operation: Operation(operation), Model: cap.Model, Incoming: cloneObject(input), MaterialRefs: map[string][]any{}, Quantities: map[string][]string{}, Conditions: map[string]any{}}
	p := cloneObject(input)
	if v, present := p["public_profile_id"]; present {
		if stringValue(v) != publicProfileID(cap) {
			return r, fail(409, "estimate_configuration_changed", "Selected configuration changed; refresh the estimate")
		}
		delete(p, "public_profile_id")
	}
	for _, key := range []string{"request_blob", "material_refs", "quantities", "conditions", "billing", "account_id", "user_id", "key_id", "wire", "execution_template"} {
		if _, ok := p[key]; ok {
			return r, fail(400, "reserved_parameter", "Reserved gateway field: "+key)
		}
	}
	if m := stringValue(p["model"]); m != "" && m != cap.Model {
		return r, fail(400, "model_conflict", "Request model conflicts with capability")
	}
	if value, exists := p["mask"]; exists {
		if _, array := value.([]any); !array && value != nil {
			p["mask"] = []any{value}
		}
	}
	for _, pair := range [][2]string{{"seconds", "duration"}, {"ratio", "aspect_ratio"}, {"masks", "mask"}, {"images", "reference_images"}, {"videos", "reference_videos"}, {"audios", "reference_audios"}, {"images", "reference_image_urls"}, {"videos", "reference_video_urls"}, {"audios", "reference_audio_urls"}} {
		if err := alias(p, pair[0], pair[1]); err != nil {
			return r, err
		}
	}
	rules := mergeRuleLayers(append(append([]ParameterRule{}, cap.Template.Rules...), cap.TemplateRules...))
	var err error
	intended := map[string]any{}
	for _, kind := range []string{"images", "videos", "audios", "masks"} {
		if refs, ok := p[kind].([]any); ok && len(refs) > 0 {
			intended[kind] = refs
		}
	}
	p, err = applyParameterRules(p, rules, "semantic")
	if err != nil {
		return r, err
	}
	for kind, refs := range intended {
		if !reflect.DeepEqual(refs, p[kind]) {
			return r, fail(400, "material_integrity_failed", "Semantic rules cannot remove or modify reference intent")
		}
	}
	p["model"] = cap.UpstreamModel
	if operation == "video" {
		v, present := p["seconds"]
		if !present && cap.Duration.Default != nil {
			v = *cap.Duration.Default
		}
		n, e := requestSeconds(v)
		if e != nil {
			return r, e
		}
		p["seconds"] = n
		r.Duration = strconv.Itoa(n)
		r.Quantities["request_duration"] = []string{r.Duration}
		r.Quantities["fixed_duration"] = []string{r.Duration}
	}
	if operation == "speech" {
		text, ok := p["input"].(string)
		if !ok || text == "" {
			return r, fail(400, "speech_input_required", "Speech requires input text")
		}
		r.Quantities["input_characters"] = []string{strconv.Itoa(utf8.RuneCountInString(text))}
	}
	for _, kind := range []string{"image", "video", "audio", "mask"} {
		list, e := orderedValues(p[kind+"s"])
		if e != nil {
			return r, e
		}
		r.MaterialRefs[kind] = list
	}
	for _, file := range uploads {
		kind := strings.TrimSuffix(strings.TrimSuffix(file.Field, "[]"), "s")
		if !contains([]string{"image", "video", "audio", "mask"}, kind) || len(file.Data) > 50<<20 || len(file.Data) == 0 {
			return r, fail(400, "invalid_upload", "Invalid media upload")
		}
		r.MaterialRefs[kind] = append(r.MaterialRefs[kind], map[string]any{"url": "data:" + file.MIME + ";base64," + base64String(file.Data)})
	}
	count := 0
	for kind, refs := range r.MaterialRefs {
		count += len(refs)
		for _, ref := range refs {
			raw, form, e := refString(ref)
			if e != nil {
				return r, e
			}
			if len(raw) > 70<<20 {
				return r, fail(413, "reference_too_large", "Reference exceeds limit")
			}
			if _, e = referenceFormat(raw, form); e != nil {
				return r, e
			}
			if !strings.HasPrefix(raw, "https://") {
				mime, data, e := decodeMaterial(raw)
				if e != nil || !strings.HasPrefix(mime, kind+"/") && kind != "mask" || len(data) > 50<<20 {
					return r, fail(400, "invalid_reference_mime", "Reference content must match its media kind")
				}
			}
		}
		if len(refs) > 0 {
			p[kind+"s"] = refs
		}
	}
	if count > 16 {
		return r, fail(400, "reference_limit", "At most 16 references allowed")
	}
	r.Parameters = p
	r.InputImages = len(r.MaterialRefs["image"])
	r.Quantities["accepted_input_images"] = []string{strconv.Itoa(r.InputImages)}
	for _, key := range []string{"resolution", "size", "quality", "ratio", "n", "background", "output_format"} {
		if v, ok := p[key]; ok {
			r.Conditions[key] = v
		}
	}
	r.Conditions["has_input_image"] = r.InputImages > 0
	r.Quantities["task_count"] = []string{"1"}
	return r, nil
}

func canonicalMediaMIME(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	switch value {
	case "audio/wav", "audio/x-wav", "audio/wave", "audio/vnd.wave":
		return "audio/wav"
	case "image/jpg":
		return "image/jpeg"
	case "audio/mp3":
		return "audio/mpeg"
	}
	return value
}
func decodeMaterial(raw string) (string, []byte, error) {
	mime := ""
	if strings.HasPrefix(raw, "data:") {
		pos := strings.Index(raw, ",")
		if pos < 0 || !strings.HasSuffix(raw[:pos], ";base64") {
			return "", nil, fail(400, "invalid_reference", "Full MIME Base64 Data URL required")
		}
		mime = strings.TrimSuffix(raw[5:pos], ";base64")
		raw = raw[pos+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", nil, fail(400, "invalid_reference", "Invalid Base64")
	}
	detected := http.DetectContentType(data)
	if mime == "" {
		mime = detected
	}
	if detected != "application/octet-stream" && !strings.HasPrefix(detected, "text/plain") && canonicalMediaMIME(detected) != canonicalMediaMIME(mime) {
		return "", nil, fail(400, "invalid_reference_mime", "Declared and detected MIME differ")
	}
	return mime, data, nil
}
func encodeTemplateMaterial(ctx context.Context, client *http.Client, raw, encoding string) (string, string, error) {
	if encoding == "preserve" {
		if !strings.HasPrefix(raw, "https://") {
			mime, _, err := decodeMaterial(raw)
			return raw, mime, err
		}
		return raw, "", nil
	}
	var data []byte
	var mime string
	var err error
	if strings.HasPrefix(raw, "https://") {
		if client == nil {
			return "", "", fail(422, "online_measurement_required", "Encoding this URL requires an explicit online media GET")
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if e != nil {
			return "", "", e
		}
		resp, e := client.Do(req)
		if e != nil {
			return "", "", fail(422, "reference_unavailable", "Unable to read reference")
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", "", fail(422, "reference_unavailable", "Reference GET failed")
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, (50<<20)+1))
		mime = strings.Split(resp.Header.Get("Content-Type"), ";")[0]
		if mime == "" {
			mime = http.DetectContentType(data)
		}
	} else {
		mime, data, err = decodeMaterial(raw)
	}
	if err != nil || len(data) == 0 || len(data) > 50<<20 {
		return "", "", fail(400, "invalid_reference", "Media encoding failed or exceeds 50 MiB")
	}
	encoded := base64String(data)
	if encoding == "data_url" {
		encoded = "data:" + mime + ";base64," + encoded
	}
	return encoded, mime, nil
}
func expandTemplateEndpoint(ep TemplateEndpoint, vars map[string]string) (string, error) {
	path := ep.Path
	var err error
	path = pathVariable.ReplaceAllStringFunc(path, func(match string) string {
		key := match[1 : len(match)-1]
		v, ok := vars[key]
		if !ok || v == "" {
			err = fail(400, "missing_path_variable", "Missing variable: "+key)
		}
		return url.PathEscape(v)
	})
	q := url.Values{}
	for key, value := range ep.Query {
		mapped := pathVariable.ReplaceAllStringFunc(value, func(match string) string {
			v, ok := vars[match[1:len(match)-1]]
			if !ok {
				err = fail(400, "invalid_variable", "Unknown query variable")
			}
			return v
		})
		q.Set(key, mapped)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return path, err
}
func BuildTemplateRequest(ctx context.Context, client *http.Client, cap Capability, req Request) (PreparedMediaRequest, error) {
	t := cap.Template
	if t == nil {
		return PreparedMediaRequest{}, fail(422, "template_required", "Select an explicit protocol template")
	}
	endpoint := t.Submit
	if configured, exists := t.OperationSubmit[req.Operation]; exists {
		endpoint = configured
	}
	rules := mergeRuleLayers(append(append([]ParameterRule{}, t.Rules...), cap.TemplateRules...))
	p, err := applyParameterRules(req.Parameters, rules, "wire")
	if err != nil {
		return PreparedMediaRequest{}, err
	}
	vars := map[string]string{"model": cap.UpstreamModel}
	if t.Body != nil {
		value, e := templateValue(t.Body, p, vars)
		if e != nil {
			return PreparedMediaRequest{}, e
		}
		var ok bool
		p, ok = value.(map[string]any)
		if !ok || p == nil {
			return PreparedMediaRequest{}, fail(422, "invalid_template", "Template body must resolve to an object")
		}
	}
	modelTarget := t.ModelTarget
	if modelTarget == "" {
		modelTarget = "model"
	}
	if modelTarget == "-" {
		delete(p, "model")
	} else {
		if err = writePath(p, modelTarget, cap.UpstreamModel, false); err != nil {
			return PreparedMediaRequest{}, err
		}
	}
	for _, kind := range []string{"image", "video", "audio", "mask"} {
		delete(p, kind+"s")
	}
	used := map[string]int{}
	files := []Upload{}
	sentMaterials := []any{}
	for _, m := range t.Materials {
		values := []any{}
		for _, ref := range req.MaterialRefs[m.Kind] {
			raw, _, e := refString(ref)
			if e != nil {
				return PreparedMediaRequest{}, e
			}
			props := map[string]any{}
			if obj, ok := ref.(map[string]any); ok {
				for k, v := range obj {
					if k != "url" && k != "b64_json" {
						props[k] = v
					}
				}
			}
			if m.Role != "" && stringValue(props["role"]) != m.Role {
				continue
			}
			attributes := cloneObject(props)
			encoded, mime, e := encodeTemplateMaterial(ctx, client, raw, m.Encoding)
			if e != nil {
				return PreparedMediaRequest{}, e
			}
			if mime != "" && !strings.HasPrefix(mime, m.Kind+"/") && m.Kind != "mask" {
				return PreparedMediaRequest{}, fail(400, "invalid_reference_mime", "Material kind differs from MIME")
			}
			var value any
			if endpoint.Encoding == "multipart" {
				if len(props) > 0 {
					return PreparedMediaRequest{}, fail(400, "unrepresentable_reference", "Multipart file cannot express reference attributes")
				}
				if strings.HasPrefix(encoded, "https://") {
					encoded, _, e = encodeTemplateMaterial(ctx, client, encoded, "data_url")
					if e != nil {
						return PreparedMediaRequest{}, e
					}
				}
				mime, data, e := decodeMaterial(encoded)
				if e != nil {
					return PreparedMediaRequest{}, e
				}
				files = append(files, Upload{Field: m.Target, Name: fmt.Sprintf("%s-%d", m.Kind, len(files)+1), MIME: mime, Data: data})
				used[m.Kind]++
				continue
			}
			switch m.Shape {
			case "preserve":
				if len(props) == 0 {
					if _, ok := ref.(string); ok {
						value = encoded
					} else {
						props["url"] = encoded
						value = props
					}
				} else {
					props["url"] = encoded
					value = props
				}
			case "string":
				if len(props) > 0 {
					return PreparedMediaRequest{}, fail(400, "unrepresentable_reference", "String material mapping would lose role or attributes")
				}
				value = encoded
			case "url":
				props["url"] = encoded
				value = props
			case "b64_json":
				props["b64_json"] = encoded
				value = props
			case "data_mime":
				props["data"] = encoded
				props["mime_type"] = mime
				value = props
			case "parts":
				props["inlineData"] = map[string]any{"data": encoded, "mimeType": mime}
				value = props
			case "content":
				props["type"] = m.Kind + "_url"
				props[m.Kind+"_url"] = map[string]any{"url": encoded}
				value = props
			}
			for key, original := range attributes {
				serialized, ok := value.(map[string]any)
				if !ok || !reflect.DeepEqual(serialized[key], original) {
					return PreparedMediaRequest{}, fail(400, "unrepresentable_reference", "Material structure would replace reference attribute: "+key)
				}
			}
			if m.Wrapper != nil {
				wrapped, e := templateValue(m.Wrapper, map[string]any{"value": value}, vars)
				if e != nil {
					return PreparedMediaRequest{}, e
				}
				if !containsMaterialValue(wrapped, value) {
					return PreparedMediaRequest{}, fail(400, "material_integrity_failed", "Material wrapper must retain the complete serialized reference")
				}
				value = wrapped
			}
			values = append(values, value)
			sentMaterials = append(sentMaterials, value)
			used[m.Kind]++
		}
		if m.MaxCount > 0 && len(values) > m.MaxCount || m.Single && len(values) > 1 {
			return PreparedMediaRequest{}, fail(400, "template_reference_limit", "Template material count exceeded")
		}
		if len(values) == 0 {
			if m.Role == "" && endpoint.Encoding != "multipart" {
				if original, present := req.Parameters[m.Kind+"s"]; present && (original == nil || reflect.ValueOf(original).Kind() == reflect.Slice) {
					if err := writePath(p, m.Target, original, false); err != nil {
						return PreparedMediaRequest{}, err
					}
				}
			}
			continue
		}
		var value any = values
		if m.Single {
			value = values[0]
		} else if existing, present := readPath(p, m.Target); present {
			list, ok := existing.([]any)
			if !ok {
				return PreparedMediaRequest{}, fail(409, "material_target_conflict", "Material target must be an array")
			}
			value = append(list, values...)
		}
		if err = writePath(p, m.Target, value, false); err != nil {
			return PreparedMediaRequest{}, err
		}
	}
	for kind, refs := range req.MaterialRefs {
		if used[kind] != len(refs) {
			return PreparedMediaRequest{}, fail(400, "material_integrity_failed", "Template must preserve every "+kind+" reference")
		}
	}
	for _, reference := range sentMaterials {
		if !containsMaterialValue(p, reference) {
			return PreparedMediaRequest{}, fail(400, "material_integrity_failed", "Material target mappings replaced a serialized reference")
		}
	}
	if err := verifyBillingWire(t, req, p, rules); err != nil {
		return PreparedMediaRequest{}, err
	}
	path, err := expandTemplateEndpoint(endpoint, vars)
	if err != nil {
		return PreparedMediaRequest{}, err
	}
	encoding := endpoint.Encoding
	if encoding == "" {
		encoding = "json"
	}
	return PreparedMediaRequest{Method: endpoint.Method, Path: path, Encoding: encoding, Body: p, Files: files}, nil
}

// A wrapper may nest a serialized reference, but cannot count a consumed input
// as sent after replacing its media or dropping its role and extra attributes.
func containsMaterialValue(container, reference any) bool {
	if reflect.DeepEqual(container, reference) {
		return true
	}
	switch node := container.(type) {
	case map[string]any:
		for _, value := range node {
			if containsMaterialValue(value, reference) {
				return true
			}
		}
	case []any:
		for _, value := range node {
			if containsMaterialValue(value, reference) {
				return true
			}
		}
	}
	return false
}

func ParseTemplateResponse(object map[string]any, cap Capability) UpstreamResult {
	t := cap.Template
	if t.Mode == "sync_binary" {
		if binary, ok := object["_media_binary"].(map[string]any); ok {
			mime := stringValue(binary["mime"])
			data := stringValue(binary["data"])
			if strings.HasPrefix(mime, cap.MediaType+"/") && data != "" {
				return UpstreamResult{Status: "completed", Accepted: true, Artifacts: []Artifact{{Key: cap.MediaType + "1", Type: cap.MediaType, MIME: mime, Source: "data:" + mime + ";base64," + data}}}
			}
		}
	}
	r := UpstreamResult{Status: "unknown", Quantities: map[string][]string{}}
	first := func(paths []string) string {
		for _, p := range paths {
			for _, v := range extractValues(object, p) {
				if s := stringValue(v); s != "" {
					return s
				}
			}
		}
		return ""
	}
	r.ID = first(t.Response.TaskID)
	if progress, err := strconv.Atoi(first(t.Response.Progress)); err == nil && progress >= 0 && progress <= 100 {
		r.Quantities["upstream_progress"] = []string{strconv.Itoa(progress)}
	}
	state := strings.ToLower(first(t.Response.Status))
	if isExplicitTemplateFailure(state) {
		r.Status = "failed"
		r.Diagnostic = first(t.Response.Errors)
		return r
	}
	if mapped, ok := t.Response.States[state]; ok {
		r.Status = mapped
	}
	if r.Status == "failed" {
		r.Diagnostic = first(t.Response.Errors)
		return r
	}
	add := func(source string) {
		if source == "" {
			return
		}
		r.Artifacts = append(r.Artifacts, Artifact{Key: fmt.Sprintf("%s%d", cap.MediaType, len(r.Artifacts)+1), Type: cap.MediaType, MIME: t.Response.MIME, Source: source})
	}
	for _, p := range t.Response.URLs {
		for _, v := range extractValues(object, p) {
			source, ok := v.(string)
			if ok && (strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "/")) {
				add(source)
			}
		}
	}
	for _, p := range t.Response.Base64 {
		for _, v := range extractValues(object, p) {
			source, ok := v.(string)
			if !ok || source == "" {
				continue
			}
			if !strings.HasPrefix(source, "data:") {
				source = "data:" + t.Response.MIME + ";base64," + source
			}
			add(source)
		}
	}
	for _, p := range t.Response.Links {
		for _, v := range extractValues(object, p) {
			text, ok := v.(string)
			if !ok {
				continue
			}
			for _, source := range inlineReviewURL.FindAllString(text, -1) {
				if strings.HasPrefix(source, "https://") {
					add(strings.TrimRight(source, ")].,"))
				}
			}
		}
	}
	for _, rule := range t.Response.Results {
		nodes := []any{object}
		if rule.Path != "" {
			nodes = extractValues(object, rule.Path)
		}
		for _, node := range nodes {
			get := func(path string) string {
				if path == "" {
					return ""
				}
				values := extractValues(node, path)
				if len(values) == 0 {
					return ""
				}
				return stringValue(values[0])
			}
			source := get(rule.URL)
			mime := get(rule.MIME)
			if mime == "" {
				mime = t.Response.MIME
			}
			if encoded := get(rule.Base64); encoded != "" {
				source = encoded
				if !strings.HasPrefix(source, "data:") {
					source = "data:" + mime + ";base64," + source
				}
			}
			if source == "" && rule.ContentPath != "" {
				path, err := expandTemplateEndpoint(TemplateEndpoint{Path: rule.ContentPath}, map[string]string{"task_id": r.ID, "model": cap.UpstreamModel, "artifact_id": get(rule.ArtifactID)})
				if err == nil {
					source = path
				}
			}
			if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "data:") {
				add(source)
				r.Artifacts[len(r.Artifacts)-1].MIME = mime
				r.Artifacts[len(r.Artifacts)-1].Filename = safeArtifactFilename(get(rule.Filename))
			}
		}
	}
	if t.Mode == "sync_json" && len(r.Artifacts) > 0 && state == "" {
		r.Status = "completed"
	}
	if r.Status == "completed" && len(r.Artifacts) == 0 && t.ContentPath != "" && r.ID != "" {
		path, e := expandTemplateEndpoint(TemplateEndpoint{Path: t.ContentPath}, map[string]string{"task_id": r.ID, "model": cap.UpstreamModel})
		if e == nil {
			add(path)
		}
	}
	if r.ID != "" {
		r.Accepted = true
		if r.Status == "unknown" && state == "" {
			r.Status = "in_progress"
		}
	}
	if r.Status == "completed" && len(r.Artifacts) > 0 {
		r.Accepted = true
	}
	for target, path := range t.Response.Usage {
		values := extractValues(object, path)
		if len(values) > 0 {
			if _, err := number(stringValue(values[0])); err == nil {
				r.Quantities[target] = []string{stringValue(values[0])}
			}
		}
	}
	return r
}

func isExplicitTemplateFailure(state string) bool {
	switch state {
	case "failed", "failure", "error", "errored", "rejected", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func applyTemplateAuth(req *http.Request, auth TemplateAuth, key string) {
	switch auth.Mode {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+key)
	case "header":
		req.Header.Set(auth.Name, auth.Prefix+key)
	case "query":
		q := req.URL.Query()
		q.Set(auth.Name, auth.Prefix+key)
		req.URL.RawQuery = q.Encode()
	}
}
