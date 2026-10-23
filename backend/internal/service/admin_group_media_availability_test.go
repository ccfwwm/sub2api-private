//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateGroupDefaultsMediaAvailabilityToAvailable(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name: "media-group", Platform: PlatformImageVideo, RateMultiplier: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Equal(t, MediaAvailabilityAvailable, group.MediaAvailability)
	require.Equal(t, MediaAvailabilityAvailable, repo.created.MediaAvailability)
}

func TestAdminServiceSimpleModeUpdatePreservesMediaAvailability(t *testing.T) {
	repo := &groupRepoStubForAdmin{
		getByID: &Group{ID: 19, Name: "media-group", Platform: PlatformImageVideo, MediaAvailability: MediaAvailabilityAvailable},
	}
	svc := &adminServiceImpl{cfg: &config.Config{RunMode: config.RunModeSimple}, groupRepo: repo}

	updated, err := svc.UpdateGroup(context.Background(), 19, &UpdateGroupInput{
		MediaAvailability: ptrString(MediaAvailabilityUnavailable),
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, MediaAvailabilityUnavailable, updated.MediaAvailability)
	require.Equal(t, MediaAvailabilityUnavailable, repo.updated.MediaAvailability)
}

func TestAdminService_UpdateGroupMediaAvailabilityValidation(t *testing.T) {
	tests := []struct {
		name         string
		platform     string
		availability string
		wantReason   string
	}{
		{name: "sets media group unavailable", platform: PlatformImageVideo, availability: MediaAvailabilityUnavailable},
		{name: "rejects non-media group", platform: PlatformOpenAI, availability: MediaAvailabilityUnavailable, wantReason: "MEDIA_AVAILABILITY_REQUIRES_MEDIA_GROUP"},
		{name: "rejects unknown status", platform: PlatformImageVideo, availability: "maintenance", wantReason: "INVALID_MEDIA_AVAILABILITY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &groupRepoStubForAdmin{getByID: &Group{ID: 19, Name: "group", Platform: tt.platform, MediaAvailability: MediaAvailabilityAvailable}}
			svc := &adminServiceImpl{groupRepo: repo}

			updated, err := svc.UpdateGroup(context.Background(), 19, &UpdateGroupInput{MediaAvailability: ptrString(tt.availability)})
			if tt.wantReason != "" {
				require.Error(t, err)
				appErr := infraerrors.FromError(err)
				require.Equal(t, int32(http.StatusBadRequest), appErr.Code)
				require.Equal(t, tt.wantReason, appErr.Reason)
				require.Nil(t, repo.updated, "invalid updates must not be persisted")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, updated)
			require.Equal(t, MediaAvailabilityUnavailable, updated.MediaAvailability)
			require.Equal(t, MediaAvailabilityUnavailable, repo.updated.MediaAvailability)
		})
	}
}
