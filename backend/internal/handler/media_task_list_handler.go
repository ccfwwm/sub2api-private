package handler

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func parseMediaTaskFilter(c *gin.Context, userID int64, admin bool) (media.TaskListFilter, bool, error) {
	f := media.TaskListFilter{UserID: userID, Page: 1, PageSize: 20, SortOrder: "desc"}
	// Empty values still select page mode, and then fail validation.
	q, queryErr := url.ParseQuery(c.Request.URL.RawQuery)
	if queryErr != nil {
		return f, false, fmt.Errorf("invalid query parameters")
	}
	_, hasPage := q["page"]
	_, hasSize := q["page_size"]
	paged := hasPage || hasSize
	if paged {
		if _, ok := q["before"]; ok {
			return f, paged, fmt.Errorf("page/page_size cannot be combined with before")
		}
	}
	for _, p := range []struct {
		key    string
		target *int
		max    int
	}{{"page", &f.Page, 1000000}, {"page_size", &f.PageSize, 1000}} {
		if _, ok := q[p.key]; ok {
			n, e := strconv.Atoi(c.Query(p.key))
			if e != nil || n < 1 || n > p.max {
				return f, paged, fmt.Errorf("invalid %s", p.key)
			}
			*p.target = n
		}
	}
	for _, p := range []struct {
		key    string
		target *int64
	}{{"api_key_id", &f.KeyID}, {"group_id", &f.GroupID}, {"account_id", &f.AccountID}, {"user_id", &f.UserID}} {
		if c.Query(p.key) == "" {
			continue
		}
		if !admin && (p.key == "user_id" || p.key == "account_id") {
			return f, paged, fmt.Errorf("%s is administrator-only", p.key)
		}
		n, e := strconv.ParseInt(c.Query(p.key), 10, 64)
		if e != nil || n <= 0 {
			return f, paged, fmt.Errorf("invalid %s", p.key)
		}
		*p.target = n
	}
	f.TaskID, f.Model, f.MediaType, f.Status, f.Settlement = strings.TrimSpace(c.Query("task_id")), c.Query("model"), c.Query("media_type"), c.Query("status"), c.Query("settlement_state")
	for _, p := range []struct{ key, value, allowed string }{{"media_type", f.MediaType, "image,video,audio"}, {"status", f.Status, "queued,in_progress,unknown,completed,failed"}, {"settlement_state", f.Settlement, "held,pending_usage,settled,released,manual_review"}, {"sort_order", c.Query("sort_order"), "asc,desc"}} {
		valid := p.value == ""
		for _, option := range strings.Split(p.allowed, ",") {
			valid = valid || p.value == option
		}
		if !valid {
			return f, paged, fmt.Errorf("invalid %s", p.key)
		}
	}
	if sort := c.Query("sort_by"); sort != "" && sort != "created_at" {
		return f, paged, fmt.Errorf("only created_at sorting is supported")
	}
	if c.Query("sort_order") != "" {
		f.SortOrder = c.Query("sort_order")
	}
	for _, p := range []struct {
		key    string
		target **time.Time
	}{{"start_time", &f.Start}, {"end_time", &f.End}} {
		if c.Query(p.key) == "" {
			continue
		}
		v, e := time.Parse(time.RFC3339Nano, c.Query(p.key))
		if e != nil {
			return f, paged, fmt.Errorf("invalid %s; use RFC3339", p.key)
		}
		*p.target = &v
	}
	if f.Start != nil && f.End != nil && !f.Start.Before(*f.End) {
		return f, paged, fmt.Errorf("start_time must precede end_time")
	}
	if len(f.TaskID) > 200 || len(f.Model) > 300 {
		return f, paged, fmt.Errorf("task or model filter is too long")
	}
	return f, paged, nil
}

func (h *MediaHandler) listTasks(c *gin.Context, userID int64, admin bool) {
	f, paged, err := parseMediaTaskFilter(c, userID, admin)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var tasks []media.StoredTask
	var total int64
	if paged {
		tasks, total, err = h.Engine.Store.TaskPage(c.Request.Context(), f)
	} else {
		tasks, err = h.Engine.Store.TaskList(c.Request.Context(), f.UserID, 50, c.Query("before"))
	}
	if err != nil {
		mediaError(c, err)
		return
	}
	items, err := h.Engine.Store.TaskRecords(c.Request.Context(), tasks, admin)
	if err != nil {
		mediaError(c, err)
		return
	}
	if paged {
		response.Paginated(c, items, total, f.Page, f.PageSize)
	} else {
		response.Success(c, items)
	}
}
