// Package progress defines shared types for streaming task progress events
// from the container sidecar to the queue layer.
package progress

// Event is emitted by the container sidecar as a "PROGRESS <json>" line on
// stdout. Fields match the sidecar protocol defined in M6 AJ.
type Event struct {
	Iter      int    `json:"iter"`
	Action    string `json:"action"`
	Tokens    int64  `json:"tokens_cum"`
	CostCents int    `json:"cost_cents_cum"`
}

// Callback is called once per PROGRESS line successfully parsed from container
// stdout. Implementations must be safe for concurrent use.
type Callback func(ev Event)
