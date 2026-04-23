package githubcompare_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/githubcompare"
)

type staticTokener struct{ tok string }

func (s staticTokener) InstallationToken(ctx context.Context) (string, error) {
	return s.tok, nil
}

func TestCompare_ReturnsFileDiffs(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Contains(t, r.URL.Path, "/repos/alice/bob/compare/main...feature")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"files": []map[string]interface{}{
				{
					"filename": "foo.go",
					"status":   "modified",
					"patch":    "@@ -1,1 +1,1 @@\n-a\n+b\n",
				},
				{
					"filename": "old_test.go",
					"status":   "removed",
					"patch":    "@@ -1,1 +0,0 @@\n-func TestX(t *testing.T) {}\n",
				},
			},
		})
	}))
	defer gh.Close()

	c := githubcompare.New(gh.URL, staticTokener{tok: "test-token"})
	diffs, err := c.Compare(context.Background(), "alice/bob", "main", "feature")
	require.NoError(t, err)
	require.Len(t, diffs, 2)
	require.Equal(t, "foo.go", diffs[0].Path)
	require.Equal(t, []string{"a"}, diffs[0].Removed)
	require.Equal(t, []string{"b"}, diffs[0].Added)
	require.True(t, diffs[1].Deleted)
	require.Equal(t, "old_test.go", diffs[1].Path)
}

func TestCompare_HTTPError(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer gh.Close()
	c := githubcompare.New(gh.URL, staticTokener{tok: "t"})
	_, err := c.Compare(context.Background(), "x/y", "main", "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func TestCompare_EmptyFiles(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"files":[]}`)
	}))
	defer gh.Close()
	c := githubcompare.New(gh.URL, staticTokener{tok: "t"})
	diffs, err := c.Compare(context.Background(), "x/y", "main", "head")
	require.NoError(t, err)
	require.Empty(t, diffs)
}

func TestCompare_TokenerError(t *testing.T) {
	// Return an error from the tokener — Compare must propagate.
	errTokener := errorTokener{err: errStatic("token mint failed")}
	c := githubcompare.New("http://unused", errTokener)
	_, err := c.Compare(context.Background(), "x/y", "main", "head")
	require.Error(t, err)
	require.Contains(t, err.Error(), "token mint failed")
}

type errorTokener struct{ err error }

func (e errorTokener) InstallationToken(ctx context.Context) (string, error) {
	return "", e.err
}

type errStatic string

func (e errStatic) Error() string { return string(e) }
