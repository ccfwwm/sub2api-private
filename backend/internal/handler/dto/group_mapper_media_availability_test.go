package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesMediaAvailabilityWithAvailableDefault(t *testing.T) {
	group := &service.Group{ID: 7, Name: "media", Platform: service.PlatformImageVideo}

	raw, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"media_availability":"available"`)

	group.MediaAvailability = service.MediaAvailabilityUnavailable
	raw, err = json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"media_availability":"unavailable"`)
}
