// Package model contains LAM-specific domain data, not UI or RPC framework types.
package model

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

const SchemaVersion = 1

type CredentialRef struct {
	Kind       string `json:"kind"`
	Service    string `json:"service,omitempty"`
	Account    string `json:"account,omitempty"`
	Version    uint64 `json:"version,omitempty"`
	EnvKey     string `json:"env_key,omitempty"`
	ApprovalID string `json:"approval_id,omitempty"`
	ProfileID  string `json:"profile_id,omitempty"`
}
type Proxy struct {
	Mode    string `json:"mode"`
	HTTP    string `json:"http,omitempty"`
	HTTPS   string `json:"https,omitempty"`
	All     string `json:"all,omitempty"`
	NoProxy string `json:"noProxy,omitempty"`
}
type RelayIdentity struct {
	RuntimeProfileID string `json:"runtimeProfileId"`
	SourceProfileID  string `json:"sourceProfileId"`
	ProviderPolicy   string `json:"providerPolicy"`
}
type Profile struct {
	Relay             *RelayIdentity `json:"relay,omitempty"`
	QuotaCredentialID string         `json:"quotaCredentialId,omitempty"`
	TokenExpiration   *time.Time     `json:"tokenExpiration,omitempty"`
	ID                string         `json:"id"`
	Name              string         `json:"displayName"`
	Home              string         `json:"codexHome"`
	Managed           bool           `json:"managed"`
	AuthMode          string         `json:"authMode"`
	Note              string         `json:"note,omitempty"`
	RenewalDate       string         `json:"renewalDate,omitempty"`
	Proxy             Proxy          `json:"proxy"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	CredentialID      string         `json:"credentialId,omitempty"`
	AuthStatus        string         `json:"authStatus,omitempty"`
	Email             string         `json:"email,omitempty"`
	SessionCount      int            `json:"sessionCount"`
}
type Capabilities struct {
	Streaming         string `json:"streaming"`
	FunctionTools     string `json:"functionTools"`
	StructuredOutputs string `json:"structuredOutputs"`
	Reasoning         string `json:"reasoning"`
	Images            string `json:"images"`
	CustomTools       string `json:"customTools"`
	Namespaces        string `json:"namespaces"`
}

func UnknownCapabilities() Capabilities {
	return Capabilities{"unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown"}
}
func Supported(v string) bool { return v == "supported" || v == "partial" }

type ProviderModel struct {
	// CodexCatalog optionally overrides LAM 0.4.4-compatible operational defaults.
	CodexCatalog  json.RawMessage `json:"codexCatalog,omitempty"`
	ID            string          `json:"id"`
	Label         string          `json:"label,omitempty"`
	ContextWindow int64           `json:"contextWindow,omitempty"`
	Capabilities  *Capabilities   `json:"capabilities,omitempty"`
}
type CodexOptions struct {
	DisplayName         string  `json:"displayName,omitempty"`
	StreamIdleTimeoutMS *uint64 `json:"streamIdleTimeoutMs,omitempty"`
	RequestMaxRetries   *uint8  `json:"directRequestMaxRetries,omitempty"`
	StreamMaxRetries    *uint8  `json:"directStreamMaxRetries,omitempty"`
	RouteViaGateway     bool    `json:"routeViaGateway,omitempty"`
}
type Provider struct {
	Codex           CodexOptions      `json:"codex"`
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Protocol        string            `json:"protocol"`
	BaseURL         string            `json:"baseUrl"`
	Models          []ProviderModel   `json:"models"`
	DefaultModel    string            `json:"defaultModel"`
	Credential      CredentialRef     `json:"credential"`
	AuthKind        string            `json:"authKind"`
	AuthHeader      string            `json:"authHeader,omitempty"`
	EnvHeaders      map[string]string `json:"envHeaders,omitempty"`
	QueryParams     map[string]string `json:"queryParams,omitempty"`
	Capabilities    Capabilities      `json:"capabilities"`
	UpstreamPath    string            `json:"upstreamPath,omitempty"`
	Compatibility   string            `json:"compatibilityProfile,omitempty"`
	ReasoningEffort string            `json:"reasoningEffort,omitempty"`
	Revision        uint64            `json:"revision"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

func (p Provider) HasModel(id string) bool {
	for _, m := range p.Models {
		if m.ID == id {
			return true
		}
	}
	return false
}
func (p Provider) ModelCapabilities(id string) Capabilities {
	for _, m := range p.Models {
		if m.ID == id && m.Capabilities != nil {
			return *m.Capabilities
		}
	}
	return p.Capabilities
}

type Binding struct {
	CatalogPath      string        `json:"catalogPath,omitempty"`
	CatalogHash      string        `json:"catalogHash,omitempty"`
	ID               string        `json:"id"`
	ProfileID        string        `json:"profileId"`
	ProviderID       string        `json:"providerId"`
	Model            string        `json:"model"`
	Generation       uint64        `json:"generation"`
	RouteKind        string        `json:"routeKind"`
	GatewayURL       string        `json:"gatewayUrl,omitempty"`
	TokenRef         CredentialRef `json:"tokenRef,omitempty"`
	ConfigBeforeHash string        `json:"configBeforeHash,omitempty"`
	ConfigAfterHash  string        `json:"configAfterHash,omitempty"`
	ConfigBackup     string        `json:"configBackup,omitempty"`
	CreatedAt        time.Time     `json:"createdAt"`
}
type CommandApproval struct {
	ID         string    `json:"id"`
	Executable string    `json:"executable"`
	Args       []string  `json:"args"`
	SHA256     string    `json:"sha256"`
	ApprovedAt time.Time `json:"approvedAt"`
}
type GatewayLimits struct {
	MaxConcurrent        int   `json:"maxConcurrent"`
	PerBinding           int   `json:"perBinding"`
	MaxQueued            int   `json:"maxQueued"`
	PerBindingQueued     int   `json:"perBindingQueued"`
	BodyBytes            int64 `json:"bodyBytes"`
	FrameBytes           int   `json:"frameBytes"`
	ToolArgumentBytes    int   `json:"toolArgumentBytes"`
	MaxTools             int   `json:"maxTools"`
	ResponseBytes        int64 `json:"responseBytes"`
	FirstResponseSeconds int   `json:"firstResponseSeconds"`
	RequestSeconds       int   `json:"requestSeconds"`
}

func DefaultLimits() GatewayLimits {
	return GatewayLimits{32, 4, 64, 8, 4 << 20, 256 << 10, 1 << 20, 128, 32 << 20, 60, 1200}
}

type Settings struct {
	GatewayPort         int           `json:"gatewayPort,omitempty"`
	AccountMode         string        `json:"accountMode"`
	ModeAvailability    string        `json:"modeAvailability,omitempty"`
	ExpandedCardActions bool          `json:"expandedCardActions,omitempty"`
	TrayOpacity         float64       `json:"trayOpacity"`
	Theme               string        `json:"theme"`
	Language            string        `json:"language"`
	Terminal            string        `json:"terminal"`
	CodexPath           string        `json:"codexPath"`
	RefreshSeconds      int           `json:"refreshSeconds"`
	ShowDock            bool          `json:"showDock"`
	CloseToTray         bool          `json:"closeToTray"`
	OpenAtLogin         bool          `json:"openAtLogin"`
	EmbeddedTerminal    bool          `json:"embeddedTerminal"`
	ApprovalPolicy      string        `json:"approvalPolicy"`
	SandboxMode         string        `json:"sandboxMode"`
	Gateway             GatewayLimits `json:"gateway"`
	AntigravityPort     int           `json:"antigravityPort,omitempty"`
	AntigravityURL      string        `json:"antigravityUrl,omitempty"`
	AntigravityCSRF     CredentialRef `json:"antigravityCsrf,omitempty"`
	UpdateURL           string        `json:"updateUrl,omitempty"`
	UpdatePublicKey     string        `json:"updatePublicKey,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{AccountMode: "profile", ModeAvailability: "profile", TrayOpacity: 1, Theme: "system", Language: "system", Terminal: "terminal", CodexPath: "codex", RefreshSeconds: 180, ShowDock: true, CloseToTray: true, ApprovalPolicy: "on-request", SandboxMode: "workspace-write", Gateway: DefaultLimits()}
}

type PATPeriod struct {
	ProfileID   string     `json:"accountId"`
	Label       string     `json:"accountLabel"`
	WorkspaceID string     `json:"workspaceId,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
}

type PendingBinding struct {
	ProfileID        string `json:"profileId"`
	ProviderID       string `json:"providerId"`
	Model            string `json:"model"`
	RouteKind        string `json:"routeKind"`
	LegacyConfigHash string `json:"legacyConfigHash,omitempty"`
	MigrationID      string `json:"migrationId"`
}
type State struct {
	PendingBindings []PendingBinding `json:"pendingBindings,omitempty"`

	PATTimeline   []PATPeriod                `json:"patTimeline,omitempty"`
	SchemaVersion int                        `json:"schemaVersion"`
	Revision      uint64                     `json:"revision"`
	Profiles      map[string]Profile         `json:"profiles"`
	Providers     map[string]Provider        `json:"providers"`
	Bindings      map[string]Binding         `json:"bindings"`
	Approvals     map[string]CommandApproval `json:"approvals"`
	Settings      Settings                   `json:"settings"`
	Prices        []RateCard                 `json:"prices"`
	ActivePAT     string                     `json:"activePat,omitempty"`
}

func NewState() State {
	return State{SchemaVersion: SchemaVersion, Revision: 1, Profiles: map[string]Profile{}, Providers: map[string]Provider{}, Bindings: map[string]Binding{}, Approvals: map[string]CommandApproval{}, Settings: DefaultSettings(), Prices: []RateCard{}}
}
func (s *State) Normalize() {
	if s.Settings.AccountMode == "" {
		s.Settings.AccountMode = "profile"
	}
	if s.Settings.TrayOpacity == 0 {
		s.Settings.TrayOpacity = 1
	}
	if s.Profiles == nil {
		s.Profiles = map[string]Profile{}
	}
	if s.Providers == nil {
		s.Providers = map[string]Provider{}
	}
	if s.Bindings == nil {
		s.Bindings = map[string]Binding{}
	}
	if s.Approvals == nil {
		s.Approvals = map[string]CommandApproval{}
	}
}

type Tokens struct {
	Input           int64 `json:"inputTokens"`
	CachedInput     int64 `json:"cachedInputTokens"`
	Output          int64 `json:"outputTokens"`
	ReasoningOutput int64 `json:"reasoningOutputTokens"`
}

func (t Tokens) Uncached() int64 { return max(t.Input-t.CachedInput, 0) }
func (t Tokens) Total() int64    { return t.Input + t.Output }
func (t Tokens) Validate() error {
	if t.Input < 0 || t.CachedInput < 0 || t.Output < 0 || t.ReasoningOutput < 0 || t.CachedInput > t.Input || t.ReasoningOutput > t.Output || t.Input > math.MaxInt64-t.Output {
		return fmt.Errorf("inconsistent token counters")
	}
	return nil
}
func (t *Tokens) Add(v Tokens) error {
	if v.Validate() != nil {
		return fmt.Errorf("invalid token counters")
	}
	if t.Input > math.MaxInt64-v.Input || t.CachedInput > math.MaxInt64-v.CachedInput || t.Output > math.MaxInt64-v.Output || t.ReasoningOutput > math.MaxInt64-v.ReasoningOutput {
		return fmt.Errorf("token overflow")
	}
	n := Tokens{t.Input + v.Input, t.CachedInput + v.CachedInput, t.Output + v.Output, t.ReasoningOutput + v.ReasoningOutput}
	if err := n.Validate(); err != nil {
		return err
	}
	*t = n
	return nil
}

type RateCard struct {
	Model            string    `json:"model"`
	Version          string    `json:"version"`
	Currency         string    `json:"currency"`
	Source           string    `json:"source"`
	EffectiveAt      time.Time `json:"effectiveAt"`
	InputPerMillion  float64   `json:"inputPerMillion"`
	CachedPerMillion float64   `json:"cachedPerMillion"`
	OutputPerMillion float64   `json:"outputPerMillion"`
}

func (p RateCard) Cost(t Tokens) (float64, error) {
	if p.Model == "" || p.Version == "" || p.Currency != "USD" {
		return 0, fmt.Errorf("unknown or incompatible rate card")
	}
	if err := t.Validate(); err != nil {
		return 0, err
	}
	for _, x := range []float64{p.InputPerMillion, p.CachedPerMillion, p.OutputPerMillion} {
		if x < 0 || math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, fmt.Errorf("invalid rate")
		}
	}
	return (float64(t.Uncached())*p.InputPerMillion + float64(t.CachedInput)*p.CachedPerMillion + float64(t.Output)*p.OutputPerMillion) / 1e6, nil
}

type Session struct {
	ID               string    `json:"id"`
	ProfileID        string    `json:"profileId"`
	Path             string    `json:"path"`
	Title            string    `json:"title"`
	CWD              string    `json:"cwd,omitempty"`
	Model            string    `json:"model,omitempty"`
	Provider         string    `json:"provider,omitempty"`
	ModifiedAt       time.Time `json:"modifiedAt"`
	Size             int64     `json:"sizeBytes"`
	Protected        bool      `json:"protected"`
	ProtectionReason string    `json:"protectionReason,omitempty"`
	Diagnostics      []string  `json:"diagnostics,omitempty"`
}
type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}
type UsageEvent struct {
	ThreadCallIndex    int64           `json:"threadCallIndex,omitempty"`
	PreviousRecordID   string          `json:"previousRecordId,omitempty"`
	Effort             string          `json:"effort,omitempty"`
	SubagentType       string          `json:"subagentType,omitempty"`
	AgentRole          string          `json:"agentRole,omitempty"`
	AgentNickname      string          `json:"agentNickname,omitempty"`
	ModelContextWindow int64           `json:"modelContextWindow,omitempty"`
	LegacyRecordID     string          `json:"legacyRecordId,omitempty"`
	LineNumber         int64           `json:"lineNumber,omitempty"`
	CumulativeTotal    int64           `json:"cumulativeTotalTokens,omitempty"`
	ThreadKey          string          `json:"threadKey,omitempty"`
	ThreadName         string          `json:"threadName,omitempty"`
	Archived           bool            `json:"isArchived,omitempty"`
	WorkspaceID        string          `json:"workspaceId,omitempty"`
	AttributionSource  string          `json:"attributionSource,omitempty"`
	Legacy             json.RawMessage `json:"legacy,omitempty"`
	ID                 string          `json:"recordId"`
	ProfileID          string          `json:"profileId"`
	SourceFile         string          `json:"sourceFile"`
	SessionID          string          `json:"sessionId"`
	Workspace          string          `json:"workspace"`
	Model              string          `json:"model"`
	Timestamp          time.Time       `json:"timestamp"`
	Tokens             Tokens          `json:"tokens"`
	Cost               *float64        `json:"estimatedCostUsd,omitempty"`
	PriceVersion       string          `json:"priceVersion,omitempty"`
	PricingConfidence  string          `json:"pricingConfidence"`
}
type QuotaWindow struct {
	ID               string     `json:"id"`
	UsedPercent      *float64   `json:"usedPercent,omitempty"`
	RemainingPercent *float64   `json:"remainingPercent,omitempty"`
	WindowMinutes    int64      `json:"windowMinutes,omitempty"`
	ResetsAt         *time.Time `json:"resetsAt,omitempty"`
}
type Credits struct {
	AvailableCount *int       `json:"availableCount,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
}
type AntigravityBucket struct {
	ID                string   `json:"bucketId,omitempty"`
	Name              string   `json:"displayName"`
	Description       string   `json:"description,omitempty"`
	Window            string   `json:"window,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	ResetTime         string   `json:"resetTime,omitempty"`
	Disabled          bool     `json:"disabled,omitempty"`
}
type AntigravityGroup struct {
	Name        string              `json:"displayName"`
	Description string              `json:"description,omitempty"`
	Buckets     []AntigravityBucket `json:"buckets"`
}
type AntigravityModel struct {
	Label             string   `json:"label"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	ResetTime         string   `json:"resetTime,omitempty"`
}
type QuotaSnapshot struct {
	Description    string             `json:"description,omitempty"`
	Groups         []AntigravityGroup `json:"groups,omitempty"`
	Models         []AntigravityModel `json:"models,omitempty"`
	LocalPort      int                `json:"localPort,omitempty"`
	LocalProcessID int                `json:"localProcessId,omitempty"`
	Cached         bool               `json:"cached,omitempty"`
	ProfileID      string             `json:"profileId"`
	Source         string             `json:"source"`
	Status         string             `json:"status"`
	PlanType       string             `json:"planType,omitempty"`
	Windows        []QuotaWindow      `json:"windows"`
	Credits        *Credits           `json:"credits,omitempty"`
	FetchedAt      time.Time          `json:"fetchedAt"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	Stale          bool               `json:"stale"`
	ErrorCode      string             `json:"errorCode,omitempty"`
}
