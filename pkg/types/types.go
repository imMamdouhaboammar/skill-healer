package types

import "time"

// EntityType identifies what kind of agentic component this is.
type EntityType string

const (
	TypeCLI   EntityType = "cli"
	TypeMCP   EntityType = "mcp"
	TypeSkill EntityType = "skill"
	TypeAgent EntityType = "agent"
)

// Tier represents the activity frequency tier.
type Tier string

const (
	TierHot     Tier = "HOT"     // Actively and frequently used
	TierWarm    Tier = "WARM"    // Used occasionally
	TierCold    Tier = "COLD"    // Rarely used (1-2 times, long ago)
	TierDormant Tier = "DORMANT" // Never used or 0 calls
)

// Domain represents the functional domain classification.
type Domain string

const (
	DomainOrchestration Domain = "Agentic Orchestration & Harnesses"
	DomainFrontend      Domain = "UI/UX & Frontend Design"
	DomainBackend       Domain = "Backend, Databases & APIs"
	DomainDevOps        Domain = "DevOps, Cloud & Infrastructure"
	DomainTesting       Domain = "Testing, QA & Verification"
	DomainSecurity      Domain = "Security, Compliance & Guardrails"
	DomainAI            Domain = "AI / ML, LLMs & Data Science"
	DomainReverseEng    Domain = "Reverse Engineering & Diagnostics"
	DomainContent       Domain = "Content, Marketing, Media & Copywriting"
	DomainGeneral       Domain = "General & System Utility"
)

// Entity represents any tracked CLI, MCP, Skill, or Agent.
type Entity struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          EntityType `json:"type"`
	Domain        Domain     `json:"domain"`
	Path          string     `json:"path"`
	IsSymlink     bool       `json:"is_symlink"`
	SymlinkTarget string     `json:"symlink_target,omitempty"`
	Scope         string     `json:"scope"` // "global" | "project"
	CallCount     int64      `json:"call_count"`
	FirstSeen     *time.Time `json:"first_seen,omitempty"`
	LastSeen      *time.Time `json:"last_seen,omitempty"`
	Tier          Tier       `json:"tier"`
	Status        string     `json:"status"` // "active" | "standby" | "stale"
}

// TelemetryRecord represents persisted telemetry for an entity.
type TelemetryRecord struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	CallCount     int64   `json:"call_count"`
	FirstSeen     float64 `json:"first_seen"`
	LastSeen      float64 `json:"last_seen"`
	DaysSinceLast float64 `json:"days_since_last"`
	Tier          string  `json:"tier"`
}

// TelemetryDB represents the full stored telemetry database.
type TelemetryDB struct {
	Version       string                     `json:"version"`
	LastUpdated   float64                    `json:"last_updated"`
	TotalEntities int                        `json:"total_entities"`
	Entities      map[string]TelemetryRecord `json:"entities"`
}

// Pool represents the collected survey of all entities.
type Pool struct {
	Skills    map[string]Entity `json:"skills"`
	MCPs      map[string]Entity `json:"mcps"`
	CLIs      map[string]Entity `json:"clis"`
	Agents    map[string]Entity `json:"agents"`
	Total     int               `json:"total"`
	ScannedAt time.Time         `json:"scanned_at"`
}
