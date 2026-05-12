package api

// Workload kinds (closed set in v1).
const (
	WorkloadSoak   = "soak"
	WorkloadFuzz   = "fuzz"
	WorkloadReplay = "replay"
	WorkloadShrink = "shrink"
	WorkloadNone   = "none"
)

// Budget stop-on conditions (closed set in v1).
const (
	StopOnFirstDivergence = "first_divergence"
	StopOnFirstCrash      = "first_crash"
	StopOnNone            = "none"
)

// Oracle names (closed set in v1).
const (
	OracleStateDiff         = "state_diff"
	OracleConsensusLiveness = "consensus_liveness"
	OraclePeerHealth        = "peer_health"
)

// Scenario is the declarative input to `confluence run`.
// YAML tags are the public author-facing names; JSON tags are the wire shape
// (snake_case throughout).
type Scenario struct {
	APIVersion    string           `yaml:"apiVersion" json:"api_version"`
	Kind          string           `yaml:"kind" json:"kind"`
	Metadata      ScenarioMetadata `yaml:"metadata" json:"metadata"`
	Topology      Topology         `yaml:"topology" json:"topology"`
	Workload      Workload         `yaml:"workload" json:"workload"`
	Chaos         Chaos            `yaml:"chaos,omitempty" json:"chaos,omitempty"`
	Observability Observability    `yaml:"observability,omitempty" json:"observability,omitempty"`
	Budget        Budget           `yaml:"budget" json:"budget"`
	Oracles       []string         `yaml:"oracles,omitempty" json:"oracles,omitempty"`
}

type ScenarioMetadata struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type Topology struct {
	Rippled NodeGroup `yaml:"rippled" json:"rippled"`
	Goxrpl  NodeGroup `yaml:"goxrpl" json:"goxrpl"`
}

type NodeGroup struct {
	Count int    `yaml:"count" json:"count"`
	Image string `yaml:"image,omitempty" json:"image,omitempty"`
}

type Workload struct {
	Kind         string             `yaml:"kind" json:"kind"`
	TxRate       int                `yaml:"tx_rate,omitempty" json:"tx_rate,omitempty"`
	Accounts     int                `yaml:"accounts,omitempty" json:"accounts,omitempty"`
	RotateEvery  int                `yaml:"rotate_every,omitempty" json:"rotate_every,omitempty"`
	MutationRate float64            `yaml:"mutation_rate,omitempty" json:"mutation_rate,omitempty"`
	Reproducer   *WorkloadReproducer `yaml:"reproducer,omitempty" json:"reproducer,omitempty"`
}

type WorkloadReproducer struct {
	ID string `yaml:"id" json:"id"`
}

type Chaos struct {
	Schedule []ChaosEvent `yaml:"schedule,omitempty" json:"schedule,omitempty"`
}

// ChaosEvent mirrors today's `.chaos-schedule.json` entry shape.
// Fields are deliberately loose; the chaos runner validates internally.
type ChaosEvent struct {
	At     string         `yaml:"at,omitempty" json:"at,omitempty"`
	Kind   string         `yaml:"kind,omitempty" json:"kind,omitempty"`
	Target string         `yaml:"target,omitempty" json:"target,omitempty"`
	Params map[string]any `yaml:"params,omitempty" json:"params,omitempty"`
}

type Observability struct {
	Enabled         bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AlertWebhookURL string `yaml:"alert_webhook_url,omitempty" json:"alert_webhook_url,omitempty"`
}

type Budget struct {
	Duration string   `yaml:"duration" json:"duration"`
	StopOn   []string `yaml:"stop_on,omitempty" json:"stop_on,omitempty"`
}
