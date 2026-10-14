package media

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// TaskListFilter is shared by count and page queries. UserID is enforced by the handler.
type TaskListFilter struct {
	UserID, KeyID, AccountID, GroupID            int64
	TaskID, Model, MediaType, Status, Settlement string
	Start, End                                   *time.Time
	Page, PageSize                               int
	SortOrder                                    string
}

func (f TaskListFilter) predicate() (string, []any) {
	parts, args := []string{"TRUE"}, []any{}
	add := func(expr string, value any) {
		args = append(args, value)
		parts = append(parts, fmt.Sprintf(expr, len(args)))
	}
	for _, v := range []struct {
		field string
		value int64
	}{{"user_id", f.UserID}, {"api_key_id", f.KeyID}, {"account_id", f.AccountID}, {"group_id", f.GroupID}} {
		if v.value > 0 {
			add(v.field+"=$%d", v.value)
		}
	}
	for _, v := range []struct{ field, value string }{{"id", f.TaskID}, {"model", f.Model}, {"media_type", f.MediaType}, {"settlement_state", f.Settlement}} {
		if v.value != "" {
			add(v.field+"=$%d", v.value)
		}
	}
	if f.Status == "queued" {
		parts = append(parts, "state IN ('queued','submitting')")
	} else if f.Status != "" {
		add("state=$%d", f.Status)
	}
	if f.Start != nil {
		add("created_at >= $%d", *f.Start)
	}
	if f.End != nil {
		add("created_at < $%d", *f.End)
	}
	return strings.Join(parts, " AND "), args
}

func (s *Store) TaskPage(ctx context.Context, f TaskListFilter) ([]StoredTask, int64, error) {
	where, args := f.predicate()
	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_tasks WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "DESC"
	if f.SortOrder == "asc" {
		order = "ASC"
	}
	query := "SELECT " + taskSelect + " FROM media_tasks WHERE " + where + " ORDER BY created_at " + order + ",id " + order + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := []StoredTask{}
	for rows.Next() {
		item, e := scanTask(rows)
		if e != nil {
			return nil, 0, e
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

type TaskRecord struct {
	Task         Task    `json:"task"`
	MediaType    string  `json:"media_type"`
	Operation    string  `json:"operation"`
	KeyID        int64   `json:"api_key_id"`
	KeyName      string  `json:"api_key_name"`
	GroupID      int64   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	UserID       int64   `json:"user_id,omitempty"`
	UserEmail    string  `json:"user_email,omitempty"`
	AccountID    int64   `json:"account_id,omitempty"`
	AccountName  string  `json:"account_name,omitempty"`
	CostStatus   string  `json:"cost_status,omitempty"`
	CostAmount   *string `json:"cost_amount,omitempty"`
	MarginAmount *string `json:"margin_amount,omitempty"`
}

// TaskRecords fetches related names, artifacts, calculations and costs in batches.
// It deliberately never selects credentials, request blobs, or supplier URLs.
func (s *Store) TaskRecords(ctx context.Context, tasks []StoredTask, admin bool) ([]TaskRecord, error) {
	result := []TaskRecord{}
	if len(tasks) == 0 {
		return result, nil
	}
	ids, index := []string{}, map[string]int{}
	for _, t := range tasks {
		index[t.ID] = len(result)
		ids = append(ids, t.ID)
		result = append(result, TaskRecord{Task: publicTaskValue(t, []Artifact{}), MediaType: t.Snapshot.Capability.MediaType, Operation: t.Snapshot.Request.Operation, KeyID: t.Identity.KeyID, GroupID: t.Identity.GroupID})
	}
	names := `SELECT t.id,COALESCE(k.name,''),COALESCE(g.name,'') FROM media_tasks t LEFT JOIN api_keys k ON k.id=t.api_key_id LEFT JOIN groups g ON g.id=t.group_id WHERE t.id=ANY($1)`
	if admin {
		names = `SELECT t.id,COALESCE(k.name,''),COALESCE(g.name,''),t.user_id,COALESCE(u.email,''),t.account_id,COALESCE(a.name,'') FROM media_tasks t LEFT JOIN api_keys k ON k.id=t.api_key_id LEFT JOIN groups g ON g.id=t.group_id LEFT JOIN users u ON u.id=t.user_id LEFT JOIN accounts a ON a.id=t.account_id WHERE t.id=ANY($1)`
	}
	rows, err := s.DB.QueryContext(ctx, names, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, key, group, email, account string
		var userID, accountID int64
		if admin {
			err = rows.Scan(&id, &key, &group, &userID, &email, &accountID, &account)
		} else {
			err = rows.Scan(&id, &key, &group)
		}
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		r := &result[index[id]]
		r.KeyName, r.GroupName = key, group
		if admin {
			r.UserID, r.UserEmail, r.AccountID, r.AccountName = userID, email, accountID, account
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT task_id,key,media_type,mime_type,size_bytes,duration_seconds::text,available FROM media_artifacts WHERE task_id=ANY($1) ORDER BY task_id,key`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var a Artifact
		if err = rows.Scan(&id, &a.Key, &a.Type, &a.MIME, &a.Size, &a.Duration, &a.Available); err != nil {
			_ = rows.Close()
			return nil, err
		}
		a.ContentURL = fmt.Sprintf("/v1/tasks/%s/artifacts/%s/content", id, a.Key)
		r := &result[index[id]]
		r.Task.Artifacts = append(r.Task.Artifacts, a)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = id + ":finalize"
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT task_id,details FROM media_ledger_entries WHERE operation_key=ANY($1)`, pq.Array(keys))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var calc Calculation
		if json.Unmarshal(raw, &calc) == nil {
			result[index[id]].Task.Billing["calculation"] = calc
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if !admin {
		return result, nil
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT task_id,status,amount::text FROM media_cost_entries WHERE task_id=ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, status string
		var amount *string
		if err = rows.Scan(&id, &status, &amount); err != nil {
			return nil, err
		}
		r := &result[index[id]]
		r.CostStatus, r.CostAmount = status, amount
		if amount != nil && (status == "estimated" || status == "verified") {
			t := tasks[index[id]]
			accepted, _ := decimal.NewFromString(t.Accepted)
			captured, _ := decimal.NewFromString(t.Captured)
			refunded, _ := decimal.NewFromString(t.Refunded)
			expense, _ := decimal.NewFromString(*amount)
			margin := accepted.Add(captured).Sub(refunded).Sub(expense).StringFixed(8)
			r.MarginAmount = &margin
		}
	}
	return result, rows.Err()
}
