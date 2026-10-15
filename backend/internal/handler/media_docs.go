package handler

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// The source is generated from the reviewed structured frontend document. It is
// only used to bootstrap an empty database; published edits never rebuild this binary.
//
//go:embed docs/imagevideo-content.json
var imageVideoDocumentSeed []byte

const imageVideoDocumentSlug = "imagevideo"

func (h *MediaHandler) ensureImageVideoDocument(c *gin.Context) error {
	var exists bool
	ctx := c.Request.Context()
	if err := h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM public_documents WHERE slug=$1)`, imageVideoDocumentSlug).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	var content any
	if err := json.Unmarshal(imageVideoDocumentSeed, &content); err != nil {
		return err
	}
	raw, _ := json.Marshal(content)
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO public_documents(slug,published_version,published_content,draft_version,draft_content,published_at) VALUES($1,1,$2,1,$2,NOW()) ON CONFLICT DO NOTHING`, imageVideoDocumentSlug, raw); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO public_document_versions(slug,version,action,content) VALUES($1,1,'initial',$2) ON CONFLICT DO NOTHING`, imageVideoDocumentSlug, raw)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PublicImageVideoDocument serves only the last published version.
func (h *MediaHandler) PublicImageVideoDocument(c *gin.Context) {
	if err := h.ensureImageVideoDocument(c); err != nil {
		response.InternalError(c, "document unavailable")
		return
	}
	ctx := c.Request.Context()
	var version int64
	var raw []byte
	if err := h.db.QueryRowContext(ctx, `SELECT published_version,published_content FROM public_documents WHERE slug=$1`, imageVideoDocumentSlug).Scan(&version, &raw); err != nil {
		response.InternalError(c, "document unavailable")
		return
	}
	var content any
	if json.Unmarshal(raw, &content) != nil {
		response.InternalError(c, "document unavailable")
		return
	}
	response.Success(c, gin.H{"slug": imageVideoDocumentSlug, "version": version, "content": content})
}

func (h *MediaHandler) AdminImageVideoDocument(c *gin.Context) {
	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if !authed || subject.UserID <= 0 {
		response.Unauthorized(c, "Authentication required")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	if role != service.RoleAdmin {
		response.Forbidden(c, "Administrator access required")
		return
	}
	if err := h.ensureImageVideoDocument(c); err != nil {
		response.InternalError(c, "document unavailable")
		return
	}
	action := c.Param("action")
	if action == "" && c.Request.Method == http.MethodGet {
		action = "get"
	}
	ctx := c.Request.Context()
	switch action {
	case "get":
		var publishedVersion, draftVersion int64
		var pub, draft []byte
		if err := h.db.QueryRowContext(ctx, `SELECT published_version,published_content,draft_version,draft_content FROM public_documents WHERE slug=$1`, imageVideoDocumentSlug).Scan(&publishedVersion, &pub, &draftVersion, &draft); err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		response.Success(c, gin.H{"published_version": publishedVersion, "published_content": json.RawMessage(pub), "draft_version": draftVersion, "draft_content": json.RawMessage(draft)})
	case "save", "publish", "rollback":
		var req struct {
			Content         json.RawMessage `json:"content"`
			ExpectedVersion int64           `json:"expected_version"`
			Version         int64           `json:"version"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion <= 0 || (action != "rollback" && len(req.Content) == 0) {
			response.BadRequest(c, "content and expected_version are required")
			return
		}
		if action == "rollback" && req.Version <= 0 {
			response.BadRequest(c, "version is required")
			return
		}
		tx, err := h.db.BeginTx(ctx, nil)
		if err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		defer tx.Rollback()
		var draftVersion, publishedVersion int64
		if err = tx.QueryRowContext(ctx, `SELECT draft_version,published_version FROM public_documents WHERE slug=$1 FOR UPDATE`, imageVideoDocumentSlug).Scan(&draftVersion, &publishedVersion); err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		if req.ExpectedVersion != draftVersion {
			response.Error(c, http.StatusConflict, "document version conflict")
			return
		}
		content := req.Content
		version := draftVersion + 1
		kind := action
		if action == "save" {
			kind = "draft"
		}
		if action == "rollback" {
			if err = tx.QueryRowContext(ctx, `SELECT content FROM public_document_versions WHERE slug=$1 AND version=$2 AND action IN ('initial','publish','rollback') ORDER BY id DESC LIMIT 1`, imageVideoDocumentSlug, req.Version).Scan(&content); err != nil {
				response.BadRequest(c, "published version not found")
				return
			}
			kind = "rollback"
		}
		var sections []struct {
			ID     string `json:"id"`
			Group  string `json:"group"`
			Title  string `json:"title"`
			Blocks []struct {
				Kind     string     `json:"kind"`
				Text     string     `json:"text"`
				Method   string     `json:"method"`
				Path     string     `json:"path"`
				Language string     `json:"language"`
				Headers  []string   `json:"headers"`
				Rows     [][]string `json:"rows"`
			} `json:"blocks"`
		}
		if !json.Valid(content) || json.Unmarshal(content, &sections) != nil || len(sections) == 0 {
			response.BadRequest(c, "content must be a non-empty array of document sections")
			return
		}
		ids := make(map[string]bool, len(sections))
		for _, section := range sections {
			if strings.TrimSpace(section.ID) == "" || strings.TrimSpace(section.Group) == "" || strings.TrimSpace(section.Title) == "" || len(section.Blocks) == 0 {
				response.BadRequest(c, "each document section requires id, group, title and blocks")
				return
			}
			if ids[section.ID] {
				response.BadRequest(c, "document section ids must be unique")
				return
			}
			ids[section.ID] = true
			for _, block := range section.Blocks {
				switch block.Kind {
				case "heading", "text":
					if strings.TrimSpace(block.Text) == "" {
						response.BadRequest(c, "document text blocks cannot be empty")
						return
					}
				case "endpoint":
					if block.Method == "" || !strings.HasPrefix(block.Path, "/") {
						response.BadRequest(c, "document endpoint requires a method and absolute path")
						return
					}
				case "code":
					if block.Text == "" || block.Language == "" {
						response.BadRequest(c, "document code blocks require language and content")
						return
					}
				case "table":
					if len(block.Headers) == 0 || len(block.Rows) == 0 {
						response.BadRequest(c, "document tables require headers and rows")
						return
					}
					for _, row := range block.Rows {
						if len(row) != len(block.Headers) {
							response.BadRequest(c, "document table rows must match headers")
							return
						}
					}
				default:
					response.BadRequest(c, "unsupported document block kind")
					return
				}
			}
		}
		if action == "rollback" && publishedVersion <= 0 {
			response.BadRequest(c, "document has no published version")
			return
		}
		if action == "save" {
			_, err = tx.ExecContext(ctx, `UPDATE public_documents SET draft_version=$1,draft_content=$2,updated_at=NOW() WHERE slug=$3`, version, content, imageVideoDocumentSlug)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE public_documents SET draft_version=$1,draft_content=$2,published_version=$1,published_content=$2,published_at=NOW(),updated_at=NOW() WHERE slug=$3`, version, content, imageVideoDocumentSlug)
		}
		if err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO public_document_versions(slug,version,action,content,actor_user_id) VALUES($1,$2,$3,$4,$5)`, imageVideoDocumentSlug, version, kind, content, subject.UserID); err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		if err = tx.Commit(); err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		response.Success(c, gin.H{"version": version})
	case "history":
		rows, err := h.db.QueryContext(ctx, `SELECT version,action,actor_user_id,created_at FROM public_document_versions WHERE slug=$1 ORDER BY created_at DESC,id DESC LIMIT 100`, imageVideoDocumentSlug)
		if err != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		defer rows.Close()
		out := []gin.H{}
		for rows.Next() {
			var v int64
			var a string
			var actor sql.NullInt64
			var created any
			if err := rows.Scan(&v, &a, &actor, &created); err != nil {
				response.InternalError(c, "document unavailable")
				return
			}
			var actorID any
			if actor.Valid {
				actorID = actor.Int64
			}
			out = append(out, gin.H{"version": v, "action": a, "actor_user_id": actorID, "created_at": created})
		}
		if rows.Err() != nil {
			response.InternalError(c, "document unavailable")
			return
		}
		response.Success(c, out)
	default:
		response.NotFound(c, "document endpoint not found")
	}
}
