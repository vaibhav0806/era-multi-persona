package githubpr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestDefaultBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/repos/owner/repo", r.URL.Path)
		require.Equal(t, "token ghs_test", r.Header.Get("Authorization"))
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"default_branch":"master"}`))
	}))
	defer srv.Close()

	c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

	got, err := c.DefaultBranch(context.Background(), "owner/repo")
	require.NoError(t, err)
	require.Equal(t, "master", got)
}

func TestDefaultBranch_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, 404)
	}))
	defer srv.Close()

	c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

	_, err := c.DefaultBranch(context.Background(), "owner/repo")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func TestCreate_PostsCorrectBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/repos/owner/repo/pulls", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"https://github.com/owner/repo/pull/42","url":"https://api.github.com/repos/owner/repo/pulls/42"}`))
	}))
	defer srv.Close()
	c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

	pr, err := c.Create(context.Background(), githubpr.CreateArgs{
		Repo:  "owner/repo",
		Head:  "agent/1/foo",
		Base:  "main",
		Title: "[era] demo",
		Body:  "some body",
	})
	require.NoError(t, err)
	require.Equal(t, 42, pr.Number)
	require.Equal(t, "https://github.com/owner/repo/pull/42", pr.HTMLURL)
	require.Equal(t, "agent/1/foo", gotBody["head"])
	require.Equal(t, "main", gotBody["base"])
	require.Equal(t, "[era] demo", gotBody["title"])
}

func TestCreate_422Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Validation Failed","errors":[{"resource":"PullRequest","code":"invalid"}]}`, 422)
	}))
	defer srv.Close()
	c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

	_, err := c.Create(context.Background(), githubpr.CreateArgs{Repo: "o/r", Head: "h", Base: "main", Title: "t", Body: "b"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "422")
}

func TestClose_PatchesStateClosed(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "PATCH", r.Method)
		require.Equal(t, "/repos/owner/repo/pulls/42", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		require.Equal(t, "closed", gotBody["state"])
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"number":42,"state":"closed"}`))
	}))
	defer srv.Close()
	c := githubpr.New(srv.URL, &fakeTokens{tok: "ghs_test"})

	err := c.Close(context.Background(), "owner/repo", 42)
	require.NoError(t, err)
}
