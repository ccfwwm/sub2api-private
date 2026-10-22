package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func referencePriceContext(body, role string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if role != "" {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
		c.Set(string(middleware.ContextKeyUserRole), role)
	}
	c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestMediaReferencePriceRejectsUnauthorizedAndInvalid(t *testing.T) {
	for _, role := range []string{"", "user"} {
		c, w := referencePriceContext(`{}`, role)
		(&ModelPlazaHandler{}).SaveMediaReferencePrice(c)
		require.Equal(t, 403, w.Code)
	}
	for _, price := range []string{`{"amount":"-1","unit":"秒"}`, `{"amount":"NaN","unit":"秒"}`, `{"amount":"1e3","unit":"秒"}`, `{"amount":"0.123456789","unit":"秒"}`, `{"amount":"1","unit":""}`, `{"amount":"1","unit":"秒\n张"}`, `{"amount":"1000000000000","unit":"秒"}`} {
		c, w := referencePriceContext(`{"group_id":19,"model":"same","expected_version":2,"official_reference_price":`+price+`}`, "admin")
		(&ModelPlazaHandler{}).SaveMediaReferencePrice(c)
		require.Equal(t, 400, w.Code, price)
	}
}

func TestMediaReferencePriceSaveClearAndConflictPreserveMetadata(t *testing.T) {
	for _, action := range []string{"save", "clear", "conflict", "wrong-group"} {
		t.Run(action, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			price := media.PriceCard{ID: "retail", Version: "1", Purpose: "sale", Currency: "USD", MediaType: "video", Operation: "video", Status: "enabled"}
			price.ModelMatch.Kind, price.ModelMatch.Value = "exact", "same"
			price.Scope.Kind, price.Scope.Reference = "group", "19"
			h := &ModelPlazaHandler{media: &MediaHandler{db: db, Engine: &media.Engine{Config: func(context.Context, int64) ([]media.Capability, []media.PriceCard, error) {
				return []media.Capability{{Model: "same", MediaType: "video", Enabled: true, Status: "enabled", GroupIDs: []int64{19}, Operations: []string{"video"}}}, []media.PriceCard{price}, nil
			}}}}
			initial := `{"version":2,"models":{"same":"keep description"},"reference_prices":{"other":{"amount":"2","unit":"张"},"same":{"amount":"0.5","unit":"次"}}}`
			expectedVersion, group := 2, 19
			if action == "conflict" {
				expectedVersion = 1
			}
			if action == "wrong-group" {
				group = 20
			} else {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO settings").WithArgs(mediaDescriptionKey(19)).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery("SELECT value FROM settings.*FOR UPDATE").WithArgs(mediaDescriptionKey(19)).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(initial))
				if action == "conflict" {
					mock.ExpectRollback()
				} else {
					var stored mediaDescriptions
					require.NoError(t, json.Unmarshal([]byte(initial), &stored))
					stored.Version++
					if action == "clear" {
						delete(stored.ReferencePrices, "same")
					} else {
						stored.ReferencePrices["same"] = mediaReferencePrice{Amount: "0.10", Unit: "秒"}
					}
					encoded, marshalErr := json.Marshal(stored)
					require.NoError(t, marshalErr)
					mock.ExpectExec("UPDATE settings").WithArgs(mediaDescriptionKey(19), string(encoded)).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
			}
			input := `{"amount":"0.10","unit":"秒"}`
			if action == "clear" {
				input = "null"
			}
			c, w := referencePriceContext(fmt.Sprintf(`{"group_id":%d,"model":"same","expected_version":%d,"official_reference_price":%s}`, group, expectedVersion, input), "admin")
			h.SaveMediaReferencePrice(c)
			status := 200
			if action == "conflict" {
				status = 409
			}
			if action == "wrong-group" {
				status = 404
			}
			require.Equal(t, status, w.Code, w.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
