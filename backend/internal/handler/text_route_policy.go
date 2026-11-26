package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func prepareTextRoute(c *gin.Context, cfg *config.Config, session string, body []byte) {
	if cfg == nil || !service.IsTextInference(c.Request.Context()) || cfg.Gateway.TextRoutingMode == "off" || cfg.Gateway.TextRoutingMode == "" {
		return
	}
	if apiKey := getOpsAPIKey(c); apiKey != nil {
		session = strconv.FormatInt(apiKey.ID, 10) + ":" + session
	}
	pinned := gjson.GetBytes(body, "previous_response_id").String() != "" ||
		gjson.GetBytes(body, "cachedContent").String() != ""
	if !pinned && GetInboundEndpoint(c) == EndpointResponses {
		coverage := service.AnalyzeToolCallOutputContextCoverageBytes(body)
		pinned = (coverage.HasFunctionCallOutput && !coverage.ContextCoversAllCallIDs) ||
			gjson.GetBytes(body, "input.#(type==\"item_reference\")").Exists()
	}
	c.Request = c.Request.WithContext(service.WithTextRoutePolicy(c.Request.Context(), cfg.Gateway.TextRoutingMode,
		cfg.Gateway.TextRoutingPercent, session, GetInboundEndpoint(c), pinned))
}

func finishTextRoute(c *gin.Context, attempt *service.TextRouteAttempt, success bool, err error) {
	if attempt == nil {
		return
	}
	attempt.Finish(c.Request.Context(), success, err)
}

func allowTextSameAccountRetry(ctx context.Context, failure *service.UpstreamFailoverError, count, limit int) bool {
	if service.TextRouteEnforced(ctx) && (count >= min(limit, 1) ||
		failure == nil || failure.SameAccountRetryDelay > time.Second || service.IsTextRouteAccessFailure(failure)) {
		return false
	}
	return sameAccountRetryAllowed(failure, count, limit)
}
