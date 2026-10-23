package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Display metadata deliberately lives outside executable capability snapshots.
type mediaDescriptions struct {
	Version         int64                          `json:"version"`
	Models          map[string]string              `json:"models"`
	ReferencePrices map[string]mediaReferencePrice `json:"reference_prices,omitempty"`
	Availability    map[string]string              `json:"availability,omitempty"`
}

const (
	mediaModelAvailabilityAvailable   = "available"
	mediaModelAvailabilityUnavailable = "unavailable"
)

func normalizeMediaModelAvailability(value string) string {
	if value == mediaModelAvailabilityUnavailable {
		return mediaModelAvailabilityUnavailable
	}
	return mediaModelAvailabilityAvailable
}

// Reference prices are display metadata in USD, never executable billing rules.
type mediaReferencePrice struct {
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
}

var mediaReferenceAmount = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$`)

func mediaDescriptionKey(group int64) string {
	return fmt.Sprintf("media_model_descriptions:%d", group)
}

func (h *ModelPlazaHandler) attachDescriptions(ctx context.Context, groups []modelPlazaGroup) error {
	if h.media == nil || h.media.db == nil {
		return nil
	}
	for i := range groups {
		for j := range groups[i].MediaModels {
			// Historical groups may have no settings row or no availability map;
			// keep the public contract explicit and default each model to available.
			groups[i].MediaModels[j].Availability = mediaModelAvailabilityAvailable
		}
		var raw string
		err := h.media.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=$1", mediaDescriptionKey(groups[i].ID)).Scan(&raw)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		var descriptions mediaDescriptions
		if err = json.Unmarshal([]byte(raw), &descriptions); err != nil {
			return err
		}
		groups[i].DescriptionVersion = descriptions.Version
		for j := range groups[i].MediaModels {
			model := groups[i].MediaModels[j].Model
			groups[i].MediaModels[j].Availability = normalizeMediaModelAvailability(descriptions.Availability[model])
			groups[i].MediaModels[j].DisplayDescription = descriptions.Models[model]
			if price, ok := descriptions.ReferencePrices[model]; ok {
				groups[i].MediaModels[j].OfficialReferencePrice = &price
			}
		}
	}
	return nil
}

// PublicMediaCatalog shares visibility and price policy with the plaza, without
// inheriting the text-model plaza's optional login requirement.
func (h *ModelPlazaHandler) PublicMediaCatalog(c *gin.Context) {
	if h.media == nil {
		response.Error(c, 503, "Media catalog unavailable")
		return
	}
	groups, err := h.plazaService.ListActiveGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var allowed map[int64]struct{}
	var restricted bool
	var rates map[int64]float64
	subject, authed := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	if authed && role != "admin" {
		allowed, restricted, err = h.apiKeyService.GetUserGroupVisibility(c.Request.Context(), subject.UserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if authed {
		rates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if role != "admin" {
		groups = filterPlazaVisibleGroups(groups, allowed, restricted)
	}
	out := []modelPlazaGroup{}
	for i := range groups {
		dto := toModelPlazaGroupDTO(&groups[i], rates)
		dto.Models = []modelPlazaModel{}
		out = append(out, dto)
	}
	out, partial := attachModelPlazaMediaCatalog(c.Request.Context(), out, h.media.Engine.Catalog, c.GetString("request_id"), "")
	if err = h.attachDescriptions(c.Request.Context(), out); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	visible := []modelPlazaGroup{}
	for _, group := range out {
		if len(group.MediaModels) > 0 {
			visible = append(visible, group)
		}
	}
	var notices []string
	if partial {
		notices = []string{modelPlazaPartialMediaNotice}
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, modelPlazaResponse{Groups: visible, Notices: notices})
}

func (h *ModelPlazaHandler) SaveMediaDescription(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	var input struct {
		GroupID         int64  `json:"group_id"`
		Model           string `json:"model"`
		Description     string `json:"description"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&input) != nil || input.GroupID <= 0 || input.Model == "" || len(input.Model) > 300 || input.ExpectedVersion < 0 || utf8.RuneCountInString(input.Description) > 2000 {
		response.BadRequest(c, "Invalid model description (maximum 2000 characters)")
		return
	}
	input.Description = strings.TrimSpace(input.Description)
	version, ok := h.updateMediaDisplay(c, input.GroupID, input.Model, input.ExpectedVersion, func(metadata *mediaDescriptions) {
		if metadata.Models == nil {
			metadata.Models = map[string]string{}
		}
		if input.Description == "" {
			delete(metadata.Models, input.Model)
		} else {
			metadata.Models[input.Model] = input.Description
		}
	})
	if ok {
		response.Success(c, gin.H{"description_version": version, "display_description": input.Description})
	}
}

func (h *ModelPlazaHandler) SaveMediaReferencePrice(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	var input struct {
		GroupID         int64                `json:"group_id"`
		Model           string               `json:"model"`
		Price           *mediaReferencePrice `json:"official_reference_price"`
		ExpectedVersion int64                `json:"expected_version"`
	}
	if c.ShouldBindJSON(&input) != nil || input.GroupID <= 0 || input.Model == "" || len(input.Model) > 300 || input.ExpectedVersion < 0 {
		response.BadRequest(c, "Invalid reference price request")
		return
	}
	if input.Price != nil {
		input.Price.Amount = strings.TrimSpace(input.Price.Amount)
		input.Price.Unit = strings.TrimSpace(input.Price.Unit)
		if !mediaReferenceAmount.MatchString(input.Price.Amount) || input.Price.Unit == "" || utf8.RuneCountInString(input.Price.Unit) > 40 || strings.ContainsFunc(input.Price.Unit, unicode.IsControl) {
			response.BadRequest(c, "Use a nonnegative USD decimal (up to 12 integer and 8 decimal digits) and a unit of at most 40 characters")
			return
		}
	}
	version, ok := h.updateMediaDisplay(c, input.GroupID, input.Model, input.ExpectedVersion, func(metadata *mediaDescriptions) {
		if metadata.ReferencePrices == nil {
			metadata.ReferencePrices = map[string]mediaReferencePrice{}
		}
		if input.Price == nil {
			delete(metadata.ReferencePrices, input.Model)
		} else {
			metadata.ReferencePrices[input.Model] = *input.Price
		}
	})
	if ok {
		response.Success(c, gin.H{"description_version": version, "official_reference_price": input.Price})
	}
}

// SaveMediaAvailability changes the display-only availability for one media
// model within one group. It deliberately does not alter executable capability
// snapshots, routing, billing, or request admission.
func (h *ModelPlazaHandler) SaveMediaAvailability(c *gin.Context) {
	if !requireMediaAdmin(c) {
		return
	}
	var input struct {
		GroupID         int64  `json:"group_id"`
		Model           string `json:"model"`
		Availability    string `json:"availability"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&input) != nil || input.GroupID <= 0 || input.Model == "" || len(input.Model) > 300 || input.ExpectedVersion < 0 || (input.Availability != mediaModelAvailabilityAvailable && input.Availability != mediaModelAvailabilityUnavailable) {
		response.BadRequest(c, "Invalid model availability request")
		return
	}
	version, ok := h.updateMediaDisplay(c, input.GroupID, input.Model, input.ExpectedVersion, func(metadata *mediaDescriptions) {
		if metadata.Availability == nil {
			metadata.Availability = map[string]string{}
		}
		metadata.Availability[input.Model] = input.Availability
	})
	if ok {
		response.Success(c, gin.H{"description_version": version, "availability": input.Availability})
	}
}

func (h *ModelPlazaHandler) updateMediaDisplay(c *gin.Context, groupID int64, model string, expectedVersion int64, update func(*mediaDescriptions)) (int64, bool) {
	if h.media == nil || h.media.db == nil {
		response.Error(c, 503, "Media catalog unavailable")
		return 0, false
	}
	entries, err := h.media.Engine.Catalog(c.Request.Context(), groupID)
	if err != nil {
		mediaError(c, err)
		return 0, false
	}
	found := false
	for _, entry := range entries {
		found = found || entry.Model == model
	}
	if !found {
		response.NotFound(c, "Model is not available in this group")
		return 0, false
	}
	tx, err := h.media.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	defer func() { _ = tx.Rollback() }()
	key := mediaDescriptionKey(groupID)
	_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO settings (key,value,updated_at) VALUES ($1,'{"version":0,"models":{}}',now()) ON CONFLICT (key) DO NOTHING`, key)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	var raw string
	if err = tx.QueryRowContext(c.Request.Context(), "SELECT value FROM settings WHERE key=$1 FOR UPDATE", key).Scan(&raw); err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	var descriptions mediaDescriptions
	if err = json.Unmarshal([]byte(raw), &descriptions); err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	if descriptions.Version != expectedVersion {
		response.Error(c, http.StatusConflict, "Model display metadata changed; refresh before saving")
		return 0, false
	}
	update(&descriptions)
	descriptions.Version++
	encoded, err := json.Marshal(descriptions)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	_, err = tx.ExecContext(c.Request.Context(), "UPDATE settings SET value=$2,updated_at=now() WHERE key=$1", key, string(encoded))
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	if err = tx.Commit(); err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	return descriptions.Version, true
}
