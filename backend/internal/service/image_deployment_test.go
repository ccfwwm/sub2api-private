package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImageDeploymentRequiresInstalledController(t *testing.T) {
	t.Setenv("IMAGE_DEPLOY_SOCKET", "")
	_, err := CallImageDeployment(context.Background(), &ImageDeploymentRequest{Action: "deploy", Confirm: true})
	require.ErrorContains(t, err, "operator approval")
}
