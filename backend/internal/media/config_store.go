package media

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

type ConfigRecord struct {
	ID               string          `json:"id"`
	Version          int64           `json:"version"`
	PublishedVersion *int64          `json:"published_version"`
	Status           string          `json:"status"`
	Config           json.RawMessage `json:"config"`
	BuiltinCurrent   json.RawMessage `json:"builtin_current,omitempty"`
	PublishedConfig  json.RawMessage `json:"published_config,omitempty"`
}
type Store struct {
	templateSeed      sync.Once
	templateSeedError error
	DB                *sql.DB
	LegacyBalanceHeld func(context.Context, int64) (string, error)
	// RuntimeCapabilities projects account-mode media capabilities for binding
	// validation. Persisted capability profiles remain validated as well.
	RuntimeCapabilities func(context.Context, int64) ([]Capability, error)
}

func (s *Store) Configs(ctx context.Context, kind string, published bool) ([]ConfigRecord, error) {
	join := "v.version=h.version"
	if published {
		join = "v.version=h.published_version"
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT h.id,h.version,h.published_version,v.status,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND `+join+` WHERE h.kind=$1 ORDER BY h.id`, kind)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	list := []ConfigRecord{}
	for rows.Next() {
		var r ConfigRecord
		if e = rows.Scan(&r.ID, &r.Version, &r.PublishedVersion, &r.Status, &r.Config); e != nil {
			return nil, e
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
func (s *Store) SaveConfig(ctx context.Context, kind, id string, expected, actor int64, data json.RawMessage, publish bool) (ConfigRecord, error) {
	return s.saveConfig(ctx, kind, id, expected, actor, data, publish, nil)
}
func (s *Store) SaveProtocolConfig(ctx context.Context, kind, id string, expected, actor int64, data json.RawMessage, publish bool, upgrades []string) (ConfigRecord, error) {
	return s.saveConfig(ctx, kind, id, expected, actor, data, publish, upgrades)
}
func (s *Store) saveConfig(ctx context.Context, kind, id string, expected, actor int64, data json.RawMessage, publish bool, upgrades []string) (ConfigRecord, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return ConfigRecord{}, e
	}
	defer func() { _ = tx.Rollback() }()
	_, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "media-config")
	if e != nil {
		return ConfigRecord{}, e
	}
	var current int64
	var published *int64
	e = tx.QueryRowContext(ctx, `SELECT version,published_version FROM media_config_heads WHERE kind=$1 AND id=$2 FOR UPDATE`, kind, id).Scan(&current, &published)
	if e != nil && e != sql.ErrNoRows {
		return ConfigRecord{}, e
	}
	if current != expected {
		return ConfigRecord{}, fail(409, "version_conflict", "Refresh before editing configuration")
	}
	status := "draft"
	if publish {
		status = "enabled"
	}
	var object map[string]any
	if e = json.Unmarshal(data, &object); e != nil {
		return ConfigRecord{}, e
	}
	object["version"] = fmt.Sprint(current + 1)
	object["status"] = status
	if kind == "capability" {
		object["enabled"] = publish
	}
	data, e = json.Marshal(object)
	if e != nil {
		return ConfigRecord{}, e
	}
	switch kind {
	case "capability":
		var cap Capability
		if e = decodeConfig(data, &cap); e != nil {
			return ConfigRecord{}, e
		}
		if cap.ID != id {
			return ConfigRecord{}, fail(400, "config_identity_conflict", "Profile ID differs from configuration ID")
		}
		if e = ValidateCapability(cap, publish); e != nil {
			return ConfigRecord{}, e
		}
	case "template":
		var template ProtocolTemplate
		if e = decodeConfig(data, &template); e != nil {
			return ConfigRecord{}, e
		}
		if template.ID != id {
			return ConfigRecord{}, fail(400, "config_identity_conflict", "Template ID differs")
		}
		if e = ValidateTemplate(template, publish); e != nil {
			return ConfigRecord{}, e
		}
	case "binding":
		var binding ModelBinding
		if e = decodeConfig(data, &binding); e != nil {
			return ConfigRecord{}, e
		}
		if binding.ID != id {
			return ConfigRecord{}, fail(400, "config_identity_conflict", "Binding ID differs")
		}
		if e = ValidateBinding(binding, publish); e != nil {
			return ConfigRecord{}, e
		}
		if publish {
			if e = s.validateBindingPublication(ctx, tx, binding); e != nil {
				return ConfigRecord{}, e
			}
		}
	case "price":
		var card PriceCard
		if e = decodeConfig(data, &card); e != nil {
			return ConfigRecord{}, e
		}
		if card.ID != id {
			return ConfigRecord{}, fail(400, "config_identity_conflict", "Price ID differs from configuration ID")
		}
		if e = ValidatePriceCard(card, publish); e != nil {
			return ConfigRecord{}, e
		}
	default:
		return ConfigRecord{}, fail(400, "invalid_config_kind", "capability, price, template or binding required")
	}
	if publish && kind == "price" {
		records, e := configRecords(ctx, tx, "price", true)
		if e != nil {
			return ConfigRecord{}, e
		}
		var fresh PriceCard
		_ = json.Unmarshal(data, &fresh)
		for _, record := range records {
			if record.ID == id {
				continue
			}
			var other PriceCard
			_ = json.Unmarshal(record.Config, &other)
			if other.Status == "enabled" && other.Purpose == fresh.Purpose && other.Operation == fresh.Operation && other.MediaType == fresh.MediaType && other.Scope == fresh.Scope && other.ModelMatch == fresh.ModelMatch {
				return ConfigRecord{}, fail(409, "price_conflict", "An equivalent published price already exists")
			}
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_config_versions(kind,id,version,status,config,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, kind, id, current+1, status, []byte(data), actor)
	if e != nil {
		return ConfigRecord{}, e
	}
	newPublished := published
	if publish {
		v := current + 1
		newPublished = &v
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO media_config_heads(kind,id,version,published_version) VALUES($1,$2,$3,$4) ON CONFLICT(kind,id) DO UPDATE SET version=excluded.version,published_version=excluded.published_version`, kind, id, current+1, newPublished)
	if e != nil {
		return ConfigRecord{}, e
	}
	if publish && kind == "template" && len(upgrades) > 0 {
		if e = s.upgradeTemplateBindings(ctx, tx, id, current+1, actor, upgrades); e != nil {
			return ConfigRecord{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return ConfigRecord{}, e
	}
	return ConfigRecord{ID: id, Version: current + 1, PublishedVersion: newPublished, Status: status, Config: data}, nil
}

func decodeConfig(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fail(400, "invalid_config_fields", "Configuration has unknown fields or invalid types")
	}
	return nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func configRecords(ctx context.Context, q queryer, kind string, published bool) ([]ConfigRecord, error) {
	join := "v.version=h.version"
	if published {
		join = "v.version=h.published_version"
	}
	rows, e := q.QueryContext(ctx, `SELECT h.id,h.version,h.published_version,v.status,v.config FROM media_config_heads h JOIN media_config_versions v ON v.kind=h.kind AND v.id=h.id AND `+join+` WHERE h.kind=$1 ORDER BY h.id`, kind)
	if e != nil {
		return nil, e
	}
	defer func() { _ = rows.Close() }()
	list := []ConfigRecord{}
	for rows.Next() {
		var r ConfigRecord
		if e = rows.Scan(&r.ID, &r.Version, &r.PublishedVersion, &r.Status, &r.Config); e != nil {
			return nil, e
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
func (s *Store) Published(ctx context.Context) ([]Capability, []PriceCard, error) {
	cr, e := s.Configs(ctx, "capability", true)
	if e != nil {
		return nil, nil, e
	}
	pr, e := s.Configs(ctx, "price", true)
	if e != nil {
		return nil, nil, e
	}
	caps := []Capability{}
	prices := []PriceCard{}
	for _, r := range cr {
		var c Capability
		if e = json.Unmarshal(r.Config, &c); e != nil {
			return nil, nil, e
		}
		caps = append(caps, c)
	}
	for _, r := range pr {
		var c PriceCard
		if e = json.Unmarshal(r.Config, &c); e != nil {
			return nil, nil, e
		}
		prices = append(prices, c)
	}
	return caps, prices, nil
}
