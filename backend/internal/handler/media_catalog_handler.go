package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Display metadata deliberately lives outside executable capability snapshots.
type mediaDescriptions struct {
	Version int64             `json:"version"`
	Models  map[string]string `json:"models"`
}

func mediaDescriptionKey(group int64) string {
	return fmt.Sprintf("media_model_descriptions:%d", group)
}

func (h *ModelPlazaHandler) attachDescriptions(ctx context.Context, groups []modelPlazaGroup) error {
	if h.media == nil || h.media.db == nil {
		return nil
	}
	for i := range groups {
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
			groups[i].MediaModels[j].DisplayDescription = descriptions.Models[groups[i].MediaModels[j].Model]
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
	if h.media == nil || h.media.db == nil {
		response.Error(c, 503, "Media catalog unavailable")
		return
	}
	entries, err := h.media.Engine.Catalog(c.Request.Context(), input.GroupID)
	if err != nil {
		mediaError(c, err)
		return
	}
	found := false
	for _, entry := range entries {
		found = found || entry.Model == input.Model
	}
	if !found {
		response.NotFound(c, "Model is not available in this group")
		return
	}
	tx, err := h.media.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	key := mediaDescriptionKey(input.GroupID)
	_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO settings (key,value,updated_at) VALUES ($1,'{"version":0,"models":{}}',now()) ON CONFLICT (key) DO NOTHING`, key)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var raw string
	if err = tx.QueryRowContext(c.Request.Context(), "SELECT value FROM settings WHERE key=$1 FOR UPDATE", key).Scan(&raw); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var descriptions mediaDescriptions
	if err = json.Unmarshal([]byte(raw), &descriptions); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if descriptions.Version != input.ExpectedVersion {
		response.Error(c, http.StatusConflict, "Description changed; refresh before saving")
		return
	}
	if descriptions.Models == nil {
		descriptions.Models = map[string]string{}
	}
	input.Description = strings.TrimSpace(input.Description)
	if input.Description == "" {
		delete(descriptions.Models, input.Model)
	} else {
		descriptions.Models[input.Model] = input.Description
	}
	descriptions.Version++
	encoded, err := json.Marshal(descriptions)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	_, err = tx.ExecContext(c.Request.Context(), "UPDATE settings SET value=$2,updated_at=now() WHERE key=$1", key, string(encoded))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"description_version": descriptions.Version, "display_description": input.Description})
}
