package provider

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/storage"
)

type HealthObservation struct {
	ProviderID string    `json:"providerId"`
	ObservedAt time.Time `json:"observedAt"`
	OK         bool      `json:"ok"`
	LatencyMS  int64     `json:"latencyMs"`
	ErrorCode  string    `json:"errorCode,omitempty"`
	ModelCount int       `json:"modelCount"`
}

func (s *Service) healthPath(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", fault.New("PROVIDER_INVALID", "Invalid provider ID")
	}
	return storage.SafeJoin(s.Store.Root, filepath.Join("provider-health", id+".json"))
}
func (s *Service) Health(ctx context.Context, id string) (HealthObservation, error) {
	var out HealthObservation
	if e := ctx.Err(); e != nil {
		return out, e
	}
	path, e := s.healthPath(id)
	if e != nil {
		return out, e
	}
	raw, e := storage.ReadFile(path, 1<<20)
	if e != nil {
		return out, e
	}
	if json.Unmarshal(raw, &out) != nil || out.ProviderID != id {
		return out, fault.New("HEALTH_INVALID", "Invalid provider health observation")
	}
	return out, nil
}

// TestConnection matches the original direct Responses /models health probe.
// It does not issue a generation request, infer protocol capabilities, or change
// the provider definition. Discovery remains available for Chat providers.
func (s *Service) TestConnection(ctx context.Context, id string) (HealthObservation, error) {
	out := HealthObservation{ProviderID: id, ObservedAt: time.Now().UTC()}
	started := time.Now()
	path, e := s.healthPath(id)
	if e != nil {
		return out, e
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return out, e
	}
	p, ok := st.Providers[id]
	if !ok {
		return out, fault.New("PROVIDER_NOT_FOUND", "Provider not found")
	}
	if p.Protocol != "responses" || p.UpstreamPath != "" {
		e = fault.New("PROVIDER_DIRECT_ROUTE_REQUIRED", "Upstream health validation requires a direct Responses provider")
	} else {
		models, err := s.discover(ctx, p, 15*time.Second, 1<<20)
		e = err
		out.ModelCount = len(models)
	}
	out.LatencyMS = time.Since(started).Milliseconds()
	out.OK = e == nil
	if e != nil {
		out.ErrorCode = fault.Code(e)
	}
	raw, _ := json.MarshalIndent(out, "", "  ")
	saveCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return out, errors.Join(e, storage.AtomicWrite(saveCtx, path, raw, 0600))
}
