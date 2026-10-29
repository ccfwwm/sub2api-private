// Package media implements versioned media capabilities and task-scoped billing.
// New models have no implicit capability or retail-price fallback.
package media

import (
	"encoding/json"
	"fmt"
	"time"
)

type Error struct {
	Status        int
	Code, Message string
	Details       map[string]any
	RetryAfter    int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func fail(status int, code, message string) error {
	return &Error{Status: status, Code: code, Message: message}
}

type Duration struct {
	Mode      string           `json:"mode"`
	Values    []int            `json:"values"`
	Min       *int             `json:"min"`
	Max       *int             `json:"max"`
	Default   *int             `json:"default"`
	Required  bool             `json:"required"`
	Scenarios []map[string]any `json:"scenarios"`
}
type Material struct {
	ReferenceField string   `json:"reference_field,omitempty"`
	Formats        []string `json:"formats"`
	MaxCount       *int     `json:"max_count"`
	MaxFileBytes   *int64   `json:"max_file_bytes"`
	MaxDuration    *string  `json:"max_duration_seconds"`
}
type Capability struct {
	TemplateRef      *TemplateRef        `json:"template_ref,omitempty"`
	Template         *ProtocolTemplate   `json:"execution_template,omitempty"`
	TemplateRules    []ParameterRule     `json:"template_rules,omitempty"`
	BindingVersions  []TemplateRef       `json:"binding_versions,omitempty"`
	ID               string              `json:"profile_id"`
	Version          string              `json:"version"`
	Model            string              `json:"model"`
	UpstreamModel    string              `json:"upstream_model"`
	Provider         string              `json:"provider"`
	Adapter          string              `json:"adapter"`
	RequestFormat    string              `json:"request_format,omitempty"`
	MediaType        string              `json:"media_type"`
	Enabled          bool                `json:"enabled"`
	Status           string              `json:"status"`
	ValidationStatus string              `json:"validation_status"`
	Source           string              `json:"source"`
	Scenarios        []string            `json:"scenarios"`
	Operations       []string            `json:"operations"`
	Formats          []string            `json:"formats"`
	Duration         Duration            `json:"duration"`
	Resolutions      []string            `json:"resolutions"`
	Sizes            []string            `json:"sizes,omitempty"`
	AspectRatios     []string            `json:"aspect_ratios"`
	Qualities        []string            `json:"qualities"`
	Backgrounds      []string            `json:"backgrounds,omitempty"`
	OutputFormats    []string            `json:"output_formats,omitempty"`
	ResolutionPolicy string              `json:"resolution_policy"`
	Materials        map[string]Material `json:"materials"`
	FieldMapping     map[string]string   `json:"field_mapping"`
	Constraints      []map[string]any    `json:"constraints"`
	Unverified       []string            `json:"unverified"`
	Delivery         struct {
		PollSeconds int   `json:"poll_seconds"`
		Head        *bool `json:"head"`
		Range       *bool `json:"range"`
		Refresh     *bool `json:"refresh"`
	} `json:"delivery"`
	Limits struct {
		MaxRequestBytes    *int64 `json:"max_request_bytes"`
		MaxConcurrentTasks *int   `json:"max_concurrent_tasks"`
		ObservationSeconds *int   `json:"observation_seconds"`
	} `json:"limits"`
	// Bindings reference existing account credentials. Secrets never enter snapshots.
	AccountIDs           []int64           `json:"account_ids"`
	GroupIDs             []int64           `json:"group_ids"`
	ChannelID            int64             `json:"channel_id,omitempty"`
	PriceTemplates       []string          `json:"price_templates,omitempty"`
	CostTemplates        []string          `json:"cost_templates,omitempty"`
	MusicRateOverride    *string           `json:"music_rate_override,omitempty"`
	VerificationEvidence string            `json:"verification_evidence,omitempty"`
	UsageFields          map[string]string `json:"usage_fields,omitempty"`
	ArtifactHosts        []string          `json:"artifact_hosts,omitempty"`
}
type Rounding struct {
	Mode    string `json:"mode"`
	Step    string `json:"step"`
	Minimum string `json:"minimum"`
	Scope   string `json:"scope"`
}
type Tier struct {
	UpTo  *string `json:"up_to"`
	Price *string `json:"unit_price"`
}
type Charge struct {
	Name            string   `json:"name"`
	Unit            string   `json:"unit"`
	Price           *string  `json:"unit_price"`
	QuantitySource  string   `json:"quantity_source"`
	Fallback        *string  `json:"fallback_source"`
	UsageField      *string  `json:"usage_field"`
	Trigger         string   `json:"trigger"`
	Refundable      bool     `json:"refundable"`
	Rounding        Rounding `json:"rounding"`
	Method          string   `json:"method"`
	PackageQuantity *string  `json:"package_quantity"`
	Tiers           []Tier   `json:"tiers"`
}
type Rule struct {
	Priority int            `json:"priority"`
	When     map[string]any `json:"when"`
	Charges  []Charge       `json:"charges"`
}
type PriceCard struct {
	TimeMultiplier string                           `json:"time_multiplier,omitempty"`
	ID             string                           `json:"price_id"`
	Version        string                           `json:"version"`
	Purpose        string                           `json:"purpose"`
	Status         string                           `json:"status"`
	TestOnly       bool                             `json:"test_only"`
	Currency       string                           `json:"currency"`
	MediaType      string                           `json:"media_type"`
	Operation      string                           `json:"operation"`
	ModelMatch     struct{ Kind, Value string }     `json:"model_match"`
	Scope          struct{ Kind, Reference string } `json:"scope"`
	EffectiveAt    *time.Time                       `json:"effective_at"`
	Rules          []Rule                           `json:"rules"`
	Reservation    struct {
		MaximumCharge *string           `json:"maximum_charge"`
		MaxQuantities map[string]string `json:"max_quantities"`
		Overrun       string            `json:"overrun"`
	} `json:"reservation"`
	SuccessPolicy    string `json:"success_policy"`
	MultiplierPolicy string `json:"multiplier_policy"`
	Notes            string `json:"notes"`
}
type Identity struct {
	PlatformBaseline                       map[string]string `json:"platform_baseline,omitempty"`
	UserID, KeyID, GroupID, SubscriptionID int64
	Platform                               string
	Multiplier, TimeMultiplier             string
	ImageOverride, VideoOverride           *string
}
type Request struct {
	Wire         *PreparedMediaRequest `json:"wire,omitempty"`
	Incoming     map[string]any        `json:"incoming,omitempty"`
	MaterialRefs map[string][]any      `json:"material_refs,omitempty"`
	Uploads      []Upload              `json:"uploads,omitempty"`
	Operation    string                `json:"operation"`
	Parameters   map[string]any        `json:"request"`
	Model        string                `json:"model"`
	Duration     string                `json:"duration"`
	Conditions   map[string]any        `json:"conditions"`
	Quantities   map[string][]string   `json:"quantities"`
	InputImages  int                   `json:"input_images"`
}
type Upload struct {
	Field string `json:"field"`
	Name  string `json:"name"`
	MIME  string `json:"mime"`
	Data  []byte `json:"data"`
}
type Snapshot struct {
	Inputs         *PublicInputSummary `json:"inputs,omitempty"`
	CostMultiplier string              `json:"cost_multiplier"`
	Capability     Capability          `json:"capability"`
	Sale           PriceCard           `json:"sale"`
	Cost           *PriceCard          `json:"cost"`
	SaleRule       Rule                `json:"sale_rule"`
	CostRule       *Rule               `json:"cost_rule"`
	Identity       Identity            `json:"identity"`
	Request        Request             `json:"normalized"`
	Maximum        string              `json:"maximum"`
}
type Line struct {
	Name      string   `json:"name"`
	Unit      string   `json:"unit"`
	Original  []string `json:"original"`
	Quantity  string   `json:"quantity"`
	UnitPrice string   `json:"unit_price"`
	RawAmount string   `json:"raw_amount"`
	Trigger   string   `json:"trigger"`
}
type Calculation struct {
	Currency string   `json:"currency"`
	Items    []Line   `json:"items"`
	Total    string   `json:"total"`
	Uncapped string   `json:"uncapped"`
	Missing  []string `json:"missing_usage"`
}
type Task struct {
	Submission      *Submission         `json:"-"`
	Inputs          *PublicInputSummary `json:"inputs,omitempty"`
	ID              string              `json:"id"`
	Object          string              `json:"object"`
	Model           string              `json:"model"`
	Status          string              `json:"status"`
	SubmissionPhase string              `json:"submission_phase,omitempty"`
	Progress        int                 `json:"progress"`
	CreatedAt       int64               `json:"created_at"`
	CompletedAt     *int64              `json:"completed_at,omitempty"`
	Billing         map[string]any      `json:"billing"`
	Artifacts       []Artifact          `json:"data"`
	Diagnostic      string              `json:"diagnostic,omitempty"`
}
type Artifact struct {
	Filename   string  `json:"filename,omitempty"`
	Key        string  `json:"key"`
	Type       string  `json:"type"`
	MIME       string  `json:"mime_type"`
	Size       *int64  `json:"size_bytes"`
	Duration   *string `json:"duration_seconds"`
	ContentURL string  `json:"content_url"`
	Available  bool    `json:"available"`
	// Upstream locations are internal only, never rendered in the task response.
	Source string `json:"-"`
}
type UpstreamResult struct {
	ID, Status, Diagnostic string
	UpstreamCode           string
	UpstreamMessage        string
	UpstreamRequestID      string
	UpstreamStatusCode     int
	Response               json.RawMessage
	Artifacts              []Artifact
	Quantities             map[string][]string
	Accepted               bool
	Rejected               bool
	Retryable              bool
	RetryAfter             time.Duration
}
type Account struct {
	ObserveSubmission      func(Submission) error `json:"-"`
	CostMultiplier         string
	ID                     int64
	Concurrency            int
	BaseURL, Key, Platform string
	Allowed                bool
	// TaskAPI is set only for the local durable gateway protocol, never guessed
	// for supplier requests. These fields are not part of capability snapshots.
	TaskAPI        bool
	IdempotencyKey string
}
type AccountLookup func(id int64, groupID int64) (Account, error)
type CacheInvalidate func(ctxIdentity Identity) error

func (c Capability) Matches(model, operation string, groupID int64) bool {
	if !c.Enabled || c.Status != "enabled" || c.Model != model {
		return false
	}
	opOK, groupOK := false, false
	for _, o := range c.Operations {
		if Operation(o) == Operation(operation) {
			opOK = true
		}
	}
	for _, g := range c.GroupIDs {
		if g == groupID {
			groupOK = true
		}
	}
	return opOK && groupOK
}
func Operation(kind string) string {
	if kind == "music" {
		return "audio"
	}
	return kind
}
func Object(kind string) string {
	if kind == "audio" {
		return "audio.generation"
	}
	if kind == "image" || kind == "image_edit" || kind == "gemini_image" {
		return "image.generation"
	}
	return "video"
}
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
