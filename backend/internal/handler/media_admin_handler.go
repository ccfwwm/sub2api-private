package handler

import (
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/media"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func (h *MediaHandler) AdminConfigs(c *gin.Context) {
	kind := c.Param("kind")
	if kind != "capability" && kind != "price" {
		response.BadRequest(c, "Unknown configuration kind")
		return
	}
	if id := c.Query("id"); id != "" {
		records, err := h.Engine.Store.History(c.Request.Context(), kind, id)
		if err != nil {
			mediaError(c, err)
			return
		}
		response.Success(c, records)
		return
	}
	records, err := h.Engine.Store.Configs(c.Request.Context(), kind, false)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, records)
}
func (h *MediaHandler) AdminSave(c *gin.Context) {
	if h.Engine.Config != nil {
		response.BadRequest(c, "Configure media capabilities in accounts and model prices in groups/channels")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		c.Status(401)
		return
	}
	var body struct {
		ID       string          `json:"id"`
		Expected int64           `json:"expected_version"`
		Publish  bool            `json:"publish"`
		Config   json.RawMessage `json:"config"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid configuration")
		return
	}
	if c.Param("kind") == "capability" && body.Publish {
		h.Engine.PlatformActive.Store(true)
		var cap media.Capability
		if json.Unmarshal(body.Config, &cap) != nil {
			response.BadRequest(c, "Invalid capability")
			return
		}
		for _, g := range cap.GroupIDs {
			for _, a := range cap.AccountIDs {
				account, err := h.Engine.Lookup(a, g)
				if err != nil || !account.Allowed {
					response.BadRequest(c, "Every bound account must be eligible for each configured group")
					return
				}
			}
		}
	}
	record, err := h.Engine.Store.SaveConfig(c.Request.Context(), c.Param("kind"), body.ID, body.Expected, subject.UserID, body.Config, body.Publish)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, record)
}
func (h *MediaHandler) AdminDisable(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	var body struct {
		ID       string `json:"id"`
		Expected int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid configuration")
		return
	}
	record, err := h.Engine.Store.DisableConfig(c.Request.Context(), c.Param("kind"), body.ID, body.Expected, subject.UserID)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, record)
}
func (h *MediaHandler) AdminSimulate(c *gin.Context) {
	var body struct {
		Capability     media.Capability    `json:"capability"`
		Price          media.PriceCard     `json:"price"`
		Cost           *media.PriceCard    `json:"cost"`
		Operation      string              `json:"operation"`
		Request        map[string]any      `json:"request"`
		Identity       media.Identity      `json:"identity"`
		Quantities     map[string][]string `json:"observed_quantities"`
		CostMultiplier string              `json:"cost_multiplier"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid simulator input")
		return
	}
	if err := media.ValidatePriceCard(body.Price, false); err != nil {
		mediaError(c, err)
		return
	}
	req, err := media.Normalize(body.Capability, body.Operation, body.Request)
	if err != nil {
		mediaError(c, err)
		return
	}
	if body.Identity.Multiplier == "" {
		body.Identity.Multiplier = "1"
	}
	if body.Identity.TimeMultiplier == "" {
		body.Identity.TimeMultiplier = "1"
	}
	body.Identity.Multiplier = media.EffectiveMultiplier(body.Capability, body.Identity)
	rule, err := media.MatchRule(body.Price, req.Conditions)
	if err != nil {
		mediaError(c, err)
		return
	}
	quote, maximum, err := media.Quote(body.Price, rule, req, body.Identity)
	if err != nil {
		mediaError(c, err)
		return
	}
	result := gin.H{"quote": quote, "maximum_charge": maximum, "matched_rule": rule, "conditions": req.Conditions}
	if body.Quantities != nil {
		calc, err := media.Calculate(rule, body.Quantities, body.Identity.Multiplier, body.Identity.TimeMultiplier, "all", &maximum)
		if err != nil {
			mediaError(c, err)
			return
		}
		result["settlement"] = calc
	}
	if body.Cost != nil {
		if err := media.ValidatePriceCard(*body.Cost, false); err != nil {
			mediaError(c, err)
			return
		}
		costRule, err := media.MatchRule(*body.Cost, req.Conditions)
		if err != nil {
			mediaError(c, err)
			return
		}
		q := body.Quantities
		if q == nil {
			q = req.Quantities
		}
		if body.CostMultiplier == "" {
			body.CostMultiplier = "1"
		}
		cost, err := media.Calculate(costRule, q, body.CostMultiplier, "1", "all", nil)
		if err != nil {
			mediaError(c, err)
			return
		}
		result["estimated_cost"] = cost
		if len(cost.Missing) == 0 && len(quote.Missing) == 0 {
			income, _ := decimal.NewFromString(quote.Total)
			if settlement, ok := result["settlement"].(media.Calculation); ok && len(settlement.Missing) == 0 {
				income, _ = decimal.NewFromString(settlement.Total)
			}
			expense, _ := decimal.NewFromString(cost.Total)
			result["estimated_margin"] = income.Sub(expense).StringFixed(8)
		}
	}
	response.Success(c, result)
}
func (h *MediaHandler) AdminTasks(c *gin.Context) {
	h.listTasks(c, 0, true)
}
func (h *MediaHandler) AdminRefund(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	var body struct {
		Amount   string `json:"amount"`
		Reason   string `json:"reason"`
		Expected int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid refund")
		return
	}
	err := h.Engine.Store.Refund(c.Request.Context(), c.Param("id"), c.GetHeader("Idempotency-Key"), body.Amount, body.Reason, body.Expected, subject.UserID)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
func (h *MediaHandler) AdminCost(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	var body struct {
		Amount   string `json:"amount"`
		Evidence string `json:"evidence"`
		Expected int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid cost reconciliation")
		return
	}
	if err := h.Engine.Store.VerifyCost(c.Request.Context(), c.Param("id"), body.Amount, body.Evidence, c.GetHeader("Idempotency-Key"), body.Expected, subject.UserID); err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
func (h *MediaHandler) AdminReconcile(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	var body struct {
		ID       string `json:"upstream_id"`
		Decision string `json:"decision"`
		Evidence string `json:"evidence"`
		Expected int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&body) != nil {
		response.BadRequest(c, "Invalid reconciliation")
		return
	}
	err := h.Engine.Store.Reconcile(c.Request.Context(), c.Param("id"), body.ID, body.Decision, body.Evidence, body.Expected, subject.UserID)
	if err != nil {
		mediaError(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true, "message": fmt.Sprint("Recorded ", body.Decision, "; worker will query or release once")})
}
