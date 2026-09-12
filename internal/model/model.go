package model

import "time"

const (
	ProviderAWS   = "aws"
	ProviderAzure = "azure"
	ProviderGCP   = "gcp"
)

type DriftKind string

const (
	KindDeleted    DriftKind = "deleted"
	KindCreated    DriftKind = "created"
	KindModified   DriftKind = "modified"
	KindTagChanged DriftKind = "tag_changed"
)

// Resource is the cloud-agnostic representation of a managed object.
type Resource struct {
	Address    string            `json:"address"`
	Type       string            `json:"type"`
	Provider   string            `json:"provider"`
	ID         string            `json:"id"`
	Region     string            `json:"region,omitempty"`
	Attributes map[string]any    `json:"attributes"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type Drift struct {
	Kind    DriftKind `json:"kind"`
	Address string    `json:"address"`
	Type    string    `json:"type,omitempty"`
	ID      string    `json:"id,omitempty"`
	Path    string    `json:"path,omitempty"`
	Before  any       `json:"before,omitempty"`
	After   any       `json:"after,omitempty"`
}

type Summary struct {
	Expected    int `json:"expected"`
	Actual      int `json:"actual"`
	Deleted     int `json:"deleted"`
	Created     int `json:"created"`
	Modified    int `json:"modified"`
	TagChanged  int `json:"tag_changed"`
	TotalDrifts int `json:"total_drifts"`
}

type Report struct {
	ScannedAt time.Time `json:"scanned_at"`
	StatePath string    `json:"state_path"`
	Summary   Summary   `json:"summary"`
	Drifts    []Drift   `json:"drifts"`
	Warnings  []string  `json:"warnings,omitempty"`
}

func (r *Resource) Key() string {
	if r.ID != "" {
		return r.Provider + "|" + r.Type + "|" + r.ID
	}
	return r.Provider + "|" + r.Type + "|addr:" + r.Address
}
