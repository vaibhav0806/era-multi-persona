package main

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/queue"
)

// inMemoryRegistry is a queue.PersonaRegistry impl backed by a map. It mirrors
// the *db.Repo behavior just enough for the reconcile passes: Insert returns
// queue.ErrPersonaNameTaken on duplicate names, List returns rows ordered by
// numeric token_id ASC.
type inMemoryRegistry struct {
	mu   sync.Mutex
	rows map[string]queue.Persona // keyed by name
}

func newInMemoryRegistry(_ *testing.T) *inMemoryRegistry {
	return &inMemoryRegistry{rows: map[string]queue.Persona{}}
}

func (r *inMemoryRegistry) Lookup(_ context.Context, name string) (queue.Persona, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.rows[name]; ok {
		return p, nil
	}
	return queue.Persona{}, queue.ErrPersonaNotFound
}

func (r *inMemoryRegistry) List(_ context.Context) ([]queue.Persona, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]queue.Persona, 0, len(r.rows))
	for _, p := range r.rows {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TokenID < out[j].TokenID })
	return out, nil
}

func (r *inMemoryRegistry) Insert(_ context.Context, p queue.Persona) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[p.Name]; ok {
		return queue.ErrPersonaNameTaken
	}
	r.rows[p.Name] = p
	return nil
}

func (r *inMemoryRegistry) UpdateENSSubname(_ context.Context, name, subname string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.rows[name]
	if !ok {
		return queue.ErrPersonaNotFound
	}
	p.ENSSubname = subname
	r.rows[name] = p
	return nil
}

// stubENSWriter records EnsureSubname / SetTextRecord calls for reconcileENS
// assertions. ParentName is configurable. Distinct from notifier_ens_test.go's
// stubENS (read-only) so both can coexist in package main tests.
type stubENSWriter struct {
	parent        string
	ensuredLabels []string
	textCalls     []string // formatted "label:key=value"
}

func (s *stubENSWriter) EnsureSubname(_ context.Context, label string) error {
	s.ensuredLabels = append(s.ensuredLabels, label)
	return nil
}

func (s *stubENSWriter) SetTextRecord(_ context.Context, label, key, value string) error {
	s.textCalls = append(s.textCalls, label+":"+key+"="+value)
	return nil
}

func (s *stubENSWriter) ParentName() string { return s.parent }

// stubTransferScanner is a minimal TransferScanner for reconcileFromChain tests.
type stubTransferScanner struct {
	events []TransferEvent
	err    error
}

func (s *stubTransferScanner) ScanNewMints(_ context.Context, _ int64) ([]TransferEvent, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.events, nil
}

// stubReconcilePromptStorage implements queue.PromptStorage for reconcile tests.
// (Named distinctly from any other stub elsewhere in the package.)
type stubReconcilePromptStorage struct {
	prompts  map[string]string
	fetchErr error
}

func (s *stubReconcilePromptStorage) UploadPrompt(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *stubReconcilePromptStorage) FetchPrompt(_ context.Context, uri string) (string, error) {
	if s.fetchErr != nil {
		return "", s.fetchErr
	}
	return s.prompts[uri], nil
}

func TestReconcile_DefaultSeed_InsertsBuiltins(t *testing.T) {
	registry := newInMemoryRegistry(t)
	require.NoError(t, reconcileDefaults(context.Background(), registry))

	list, err := registry.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 3)

	names := []string{}
	for _, p := range list {
		names = append(names, p.Name)
	}
	require.ElementsMatch(t, []string{"planner", "coder", "reviewer"}, names)
}

func TestReconcile_DefaultSeed_Idempotent(t *testing.T) {
	registry := newInMemoryRegistry(t)
	require.NoError(t, reconcileDefaults(context.Background(), registry))
	// Second call: every Insert hits ErrPersonaNameTaken; reconcileDefaults
	// must swallow that and return nil.
	require.NoError(t, reconcileDefaults(context.Background(), registry))

	list, err := registry.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 3)
}

func TestReconcile_ENSRetry_SkipsNonEmpty(t *testing.T) {
	registry := newInMemoryRegistry(t)
	require.NoError(t, registry.Insert(context.Background(), queue.Persona{
		TokenID:    "0",
		Name:       "planner",
		OwnerAddr:  "0xabc",
		ENSSubname: "planner.foo.eth",
	}))
	require.NoError(t, registry.Insert(context.Background(), queue.Persona{
		TokenID:    "3",
		Name:       "rust",
		OwnerAddr:  "0xabc",
		ENSSubname: "",
	}))

	ens := &stubENSWriter{parent: "foo.eth"}
	require.NoError(t, reconcileENS(context.Background(), registry, ens))

	require.NotContains(t, ens.ensuredLabels, "planner", "should skip persona that already has ens_subname")
	require.Contains(t, ens.ensuredLabels, "rust", "should retry persona missing ens_subname")

	// And the rust row should now have ens_subname persisted.
	got, err := registry.Lookup(context.Background(), "rust")
	require.NoError(t, err)
	require.Equal(t, "rust.foo.eth", got.ENSSubname)
}

func TestReconcile_TransferScan_ImportsNewMints(t *testing.T) {
	registry := newInMemoryRegistry(t)
	for _, p := range []queue.Persona{
		{TokenID: "0", Name: "planner", OwnerAddr: "0xabc"},
		{TokenID: "1", Name: "coder", OwnerAddr: "0xabc"},
		{TokenID: "2", Name: "reviewer", OwnerAddr: "0xabc"},
	} {
		require.NoError(t, registry.Insert(context.Background(), p))
	}

	scanner := &stubTransferScanner{
		events: []TransferEvent{
			{TokenID: "3", Owner: "0xabc", URI: "stub://prompt-3"},
		},
	}
	storage := &stubReconcilePromptStorage{
		prompts: map[string]string{"stub://prompt-3": "Test prompt for token 3"},
	}
	require.NoError(t, reconcileFromChain(context.Background(), registry, scanner, storage))

	got, err := registry.List(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 4)

	tokenIDs := []string{}
	for _, p := range got {
		tokenIDs = append(tokenIDs, p.TokenID)
	}
	require.Contains(t, tokenIDs, "3")
}
