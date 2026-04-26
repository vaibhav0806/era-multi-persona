package brain

// MemoryShaper produces a single observation string from one persona run.
// Implementations decide what's worth remembering — the SDK only handles
// rolling-buffer mechanics. Return "" to skip writing this turn (e.g.
// error case, redundant content).
type MemoryShaper func(in Input, out Output) string

// BareHistoryShaper is the default shaper: returns out.Text truncated at
// maxChars. Used by SDK example agents that don't need persona-specific
// observation shape.
func BareHistoryShaper(maxChars int) MemoryShaper {
	return func(_ Input, out Output) string {
		if out.Text == "" {
			return ""
		}
		if len(out.Text) <= maxChars {
			return out.Text
		}
		return out.Text[:maxChars]
	}
}
