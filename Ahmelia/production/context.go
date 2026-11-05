package production

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProductionContext is the engine-facing, domain-neutral context of the most
// recent production. It contains only production facts, not programming
// concepts. Any producer can register a project/workspace and its artifacts.
//
// Domain-specific packages may add metadata to Artifact.Metadata, but this
// context never needs to know whether an artifact is code, image, audio, 3D,
// document, executable, or another produced object.
type ProductionContext struct {
	mu sync.RWMutex

	ProductionID string
	Producer     string
	Project      ProductionProject
	Artifacts    []ProductionArtifact
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ProductionProject struct {
	ID   string
	Name string
	Path string
}

type ProductionArtifact struct {
	ID        string
	Name      string
	Path      string
	Type      string
	Producer  string
	ProjectID string
	CreatedAt time.Time
	Metadata  map[string]string

	// Artifact Fabric metadata. These fields are optional so existing
	// providers remain source-compatible while artifacts gain identity,
	// lineage and observable verification evidence.
	Version            int      `json:"version,omitempty"`
	ChecksumSHA256     string   `json:"checksum_sha256,omitempty"`
	SizeBytes          int64    `json:"size_bytes,omitempty"`
	ParentIDs          []string `json:"parent_ids,omitempty"`
	IntegrityVerified  bool     `json:"integrity_verified,omitempty"`
	IntegrityReason    string   `json:"integrity_reason,omitempty"`
	Verified           bool     `json:"verified,omitempty"`
	VerificationReason string   `json:"verification_reason,omitempty"`
}

func (p ProductionProject) Valid() bool {
	return strings.TrimSpace(p.Path) != ""
}

func (p ProductionArtifact) Valid() bool {
	return strings.TrimSpace(p.Name) != "" && strings.TrimSpace(p.Path) != ""
}

// EnrichArtifactFromPath derives observable artifact identity from the bytes
// currently present at a path. It never claims verification when the file is
// unavailable. The caller can persist the returned artifact through the
// existing ProductionContext/WorkStore authorities.
func EnrichArtifactFromPath(a ProductionArtifact) (ProductionArtifact, error) {
	if !a.Valid() {
		return ProductionArtifact{}, fmt.Errorf("production artifact: invalid artifact")
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return a, fmt.Errorf("production artifact: open %q: %w", a.Path, err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return a, fmt.Errorf("production artifact: stat %q: %w", a.Path, err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return a, fmt.Errorf("production artifact: hash %q: %w", a.Path, err)
	}
	a.ChecksumSHA256 = hex.EncodeToString(h.Sum(nil))
	a.SizeBytes = st.Size()
	if a.Version <= 0 {
		a.Version = 1
	}
	a.IntegrityVerified = true
	a.IntegrityReason = "file exists and SHA-256 was computed from current bytes"
	return a, nil
}

// EnrichArtifactIfAvailable adds observable identity when the artifact path
// is currently readable. Missing/unavailable paths are left unchanged so
// remote or future-provider artifacts remain valid without pretending they
// were verified locally.
func EnrichArtifactIfAvailable(a ProductionArtifact) ProductionArtifact {
	if _, err := os.Stat(a.Path); err != nil {
		return a
	}
	enriched, err := EnrichArtifactFromPath(a)
	if err != nil {
		return a
	}
	return enriched
}

// VerifyArtifact checks that the current file bytes still match the artifact
// identity recorded previously. This is integrity evidence, not behavioral
// verification or a semantic judgment about whether the artifact is correct
// for the user's goal.
func VerifyArtifact(a ProductionArtifact) (bool, string, error) {
	if strings.TrimSpace(a.ChecksumSHA256) == "" {
		return false, "artifact has no recorded SHA-256 identity", nil
	}
	current, err := EnrichArtifactFromPath(a)
	if err != nil {
		return false, err.Error(), nil
	}
	if !strings.EqualFold(current.ChecksumSHA256, a.ChecksumSHA256) {
		return false, "current bytes differ from recorded SHA-256", nil
	}
	return true, "current bytes match recorded SHA-256", nil
}

func (p *ProductionContext) Clear() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	*p = ProductionContext{}
}

// Set replaces the current production context with generic production facts.
// It is intentionally unaware of any domain-specific model.
func (p *ProductionContext) Set(productionID, producer string, project ProductionProject, artifacts []ProductionArtifact) {
	if p == nil {
		return
	}
	now := time.Now().UTC()
	copied := cloneArtifacts(artifacts)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ProductionID = productionID
	p.Producer = producer
	p.Project = project
	p.Artifacts = copied
	p.CreatedAt = now
	p.UpdatedAt = now
}

// RecordArtifact appends or replaces one artifact by ID/path/name. This makes
// the registry useful for productions that create artifacts incrementally.
func (p *ProductionContext) RecordArtifact(a ProductionArtifact) error {
	if p == nil {
		return fmt.Errorf("production context: nil")
	}
	if !a.Valid() {
		return fmt.Errorf("production context: artifact inválido")
	}
	a = EnrichArtifactIfAvailable(a)
	p.mu.Lock()
	defer p.mu.Unlock()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	for i := range p.Artifacts {
		existing := &p.Artifacts[i]
		if (a.ID != "" && existing.ID == a.ID) ||
			(existing.Path != "" && filepath.Clean(existing.Path) == filepath.Clean(a.Path)) ||
			strings.EqualFold(existing.Name, a.Name) {
			*existing = cloneArtifact(a)
			p.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	p.Artifacts = append(p.Artifacts, cloneArtifact(a))
	p.UpdatedAt = time.Now().UTC()
	return nil
}

func (p *ProductionContext) Snapshot() (string, string, ProductionProject, []ProductionArtifact) {
	if p == nil {
		return "", "", ProductionProject{}, nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ProductionID, p.Producer, p.Project, cloneArtifacts(p.Artifacts)
}

func (p *ProductionContext) ResolveArtifact(name string) (ProductionArtifact, bool) {
	_, _, _, artifacts := p.Snapshot()
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return ProductionArtifact{}, false
	}
	for _, a := range artifacts {
		if strings.EqualFold(a.Name, target) ||
			strings.EqualFold(filepath.Base(a.Path), target) ||
			(a.ID != "" && strings.EqualFold(a.ID, target)) {
			return a, true
		}
	}
	return ProductionArtifact{}, false
}

func (p *ProductionContext) DescribeProject() (string, bool) {
	_, _, project, _ := p.Snapshot()
	if !project.Valid() {
		return "", false
	}
	name := project.Name
	if name == "" {
		name = project.ID
	}
	if name == "" {
		name = "la producción actual"
	}
	return fmt.Sprintf("El proyecto %s está en %s.", name, project.Path), true
}

func (p *ProductionContext) DescribeArtifacts() (string, bool) {
	_, producer, project, artifacts := p.Snapshot()
	if !project.Valid() {
		return "", false
	}
	name := project.Name
	if name == "" {
		name = project.ID
	}
	if name == "" {
		name = "la producción actual"
	}
	if len(artifacts) == 0 {
		return fmt.Sprintf("La producción de %s no tiene artefactos registrados.", name), true
	}
	names := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		names = append(names, a.Name)
	}
	prefix := "En"
	if producer != "" {
		prefix = fmt.Sprintf("La producción %s dejó en", producer)
	}
	return fmt.Sprintf("%s %s: %s.", prefix, project.Path, strings.Join(names, ", ")), true
}

func cloneArtifact(a ProductionArtifact) ProductionArtifact {
	a.Metadata = cloneMetadata(a.Metadata)
	a.ParentIDs = append([]string(nil), a.ParentIDs...)
	return a
}

func cloneArtifacts(in []ProductionArtifact) []ProductionArtifact {
	if in == nil {
		return nil
	}
	out := make([]ProductionArtifact, len(in))
	for i, a := range in {
		out[i] = cloneArtifact(a)
	}
	return out
}

func cloneMetadata(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
