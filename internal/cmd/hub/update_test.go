package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForkUpdateCannotReplaceHubWithUpstream(t *testing.T) {
	cmd := newUpdateCmd()
	require.ErrorContains(t, cmd.RunE(cmd, nil), "upstream self-update is disabled")
}
