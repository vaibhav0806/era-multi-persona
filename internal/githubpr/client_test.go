package githubpr_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/githubpr"
)

type fakeTokens struct{ tok string }

func (f *fakeTokens) InstallationToken(ctx context.Context) (string, error) { return f.tok, nil }

func TestNew_DefaultsPopulated(t *testing.T) {
	c := githubpr.New("", &fakeTokens{tok: "ghs_xxx"})
	require.NotNil(t, c)
}
