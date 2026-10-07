package scenario

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// kebabRE matches lowercase kebab-case names (letters, digits, single hyphens).
// We're strict here because the name is reused as a Kurtosis enclave-name
// segment in M2/M3 and exposed in finding records — keeping it conservative
// avoids surprises across the pipeline.
var kebabRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var amendmentIDRE = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

var amendmentNameRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// These settings affect genesis or the node's network identity. They must be
// supplied through dedicated schema fields so the compiler can keep all
// validators byte-compatible.
var guardedConfigSections = map[string]struct{}{
	"server":            {},
	"port_peer":         {},
	"port_rpc":          {},
	"port_ws":           {},
	"node_db":           {},
	"database_path":     {},
	"debug_logfile":     {},
	"ips_fixed":         {},
	"validation_seed":   {},
	"validation_quorum": {},
	"validators_file":   {},
	"network_id":        {},
	"amendments":        {},
	"veto_amendments":   {},
}

// Validate runs all semantic rules over a Scenario and returns a flat list of
// api.Error values with field paths. An empty slice means the scenario is valid.
func Validate(s *api.Scenario) []api.Error {
	var errs []api.Error
	add := func(field, msg, hint string) {
		errs = append(errs, api.Error{
			Code:    api.ErrCodeScenarioInvalid,
			Message: msg,
			Field:   field,
			Hint:    hint,
		})
	}
	if s == nil {
		add("scenario", "scenario is required", "provide a Scenario document")
		return errs
	}

	if s.APIVersion != api.Version {
		add("apiVersion", fmt.Sprintf("apiVersion must be %q, got %q", api.Version, s.APIVersion), "set apiVersion: confluence/v1")
	}
	if s.Kind != "Scenario" {
		add("kind", fmt.Sprintf("kind must be \"Scenario\", got %q", s.Kind), "set kind: Scenario")
	}

	if s.Metadata.Name == "" {
		add("metadata.name", "metadata.name is required", "")
	} else if !kebabRE.MatchString(s.Metadata.Name) {
		add("metadata.name", fmt.Sprintf("metadata.name must be kebab-case (got %q)", s.Metadata.Name), "use lowercase letters, digits, and single hyphens")
	}

	if s.Topology.Rippled.Count < 0 {
		add("topology.rippled.count", "topology.rippled.count must be >= 0", "")
	}
	if s.Topology.Goxrpl.Count < 0 {
		add("topology.goxrpl.count", "topology.goxrpl.count must be >= 0", "")
	}
	totalNodes := s.Topology.Rippled.Count + s.Topology.Goxrpl.Count
	if totalNodes < 2 {
		add("topology", fmt.Sprintf("topology must declare at least 2 total nodes (got %d)", totalNodes), "set topology.rippled.count + topology.goxrpl.count between 2 and 10")
	} else if totalNodes > 10 {
		add("topology", fmt.Sprintf("topology must declare at most 10 total nodes (got %d)", totalNodes), "set topology.rippled.count + topology.goxrpl.count between 2 and 10")
	}

	validateNodeGroup("topology.rippled", s.Topology.Rippled, add)
	validateImage("topology.goxrpl.image", s.Topology.Goxrpl.Image, s.Topology.Goxrpl.Image != "", add)
	if s.Topology.Goxrpl.Entrypoint != nil {
		add("topology.goxrpl.entrypoint", "topology.goxrpl.entrypoint is not supported", "set entrypoint under topology.rippled")
	}
	if s.Topology.Goxrpl.Config != nil {
		add("topology.goxrpl.config", "topology.goxrpl.config is not supported", "set config under topology.rippled")
	}
	if s.Topology.Goxrpl.Nodes != nil {
		add("topology.goxrpl.nodes", "topology.goxrpl.nodes is not supported", "set nodes under topology.rippled")
	}

	if s.Network != nil {
		if s.Network.NetworkID != nil && (*s.Network.NetworkID < 0 || uint64(*s.Network.NetworkID) > math.MaxUint32) {
			add("network.network_id", fmt.Sprintf("network.network_id must be between 0 and %d (got %d)", math.MaxUint32, *s.Network.NetworkID), "use a non-negative 32-bit network ID")
		}
		validateAmendments("network.amendments", s.Network.Amendments, add)
		validateAmendments("network.veto_amendments", s.Network.VetoAmendments, add)
		validateAmendmentOverlap(s.Network.Amendments, s.Network.VetoAmendments, add)
	}

	customNetwork := s.Network != nil
	customRippledConfig := len(s.Topology.Rippled.Config) > 0
	for _, node := range s.Topology.Rippled.Nodes {
		if len(node.Config) > 0 {
			customRippledConfig = true
			break
		}
	}
	if customNetwork && (s.Workload.Kind != api.WorkloadNone || s.Topology.Goxrpl.Count != 0) {
		add("network", "custom network settings require workload.kind=none with topology.goxrpl.count=0", "run a rippled-only local network before changing genesis settings")
	}
	if customRippledConfig && (s.Workload.Kind != api.WorkloadNone || s.Topology.Goxrpl.Count != 0) {
		add("topology.rippled.config", "custom rippled config requires workload.kind=none with topology.goxrpl.count=0", "run a rippled-only local network before changing genesis settings")
	}

	switch s.Workload.Kind {
	case api.WorkloadSoak, api.WorkloadNone:
		// fully supported in M1
	case api.WorkloadFuzz:
		// fuzz-only (empty schedule) is not consumed by main.star yet; require chaos schedule.
		if len(s.Chaos.Schedule) == 0 {
			add("workload.kind", "workload.kind=fuzz requires chaos.schedule entries in M1", "either add chaos.schedule entries (becomes a chaos run) or pick a different workload")
		}
	case api.WorkloadShrink:
		// Shrink uses fields not yet modelled (shrink_artifact, shrink_max_step). M2+.
		add("workload.kind", "workload.kind=shrink is not yet supported by the M1 scenario schema", "use confluence shrink directly until M2 adds shrink_args")
	case api.WorkloadReplay:
		if s.Workload.Reproducer == nil || s.Workload.Reproducer.ID == "" {
			add("workload.reproducer.id", "workload.kind=replay requires reproducer.id", "set workload.reproducer.id or change workload.kind")
		}
	case "":
		add("workload.kind", "workload.kind is required", "one of: soak, fuzz, replay, none")
	default:
		add("workload.kind", fmt.Sprintf("unknown workload.kind %q", s.Workload.Kind), "one of: soak, fuzz, replay, none")
	}

	if s.Budget.Duration == "" {
		if s.Workload.Kind != api.WorkloadNone {
			add("budget.duration", "budget.duration is required", "e.g. \"10m\"")
		}
	} else if duration, err := time.ParseDuration(s.Budget.Duration); err != nil {
		add("budget.duration", fmt.Sprintf("budget.duration is not a valid Go duration: %v", err), "use values like \"30s\", \"10m\", \"2h\"")
	} else if duration <= 0 {
		add("budget.duration", "budget.duration must be positive", "use a duration greater than 0, such as \"10m\"")
	}

	allowedStopOn := map[string]bool{
		api.StopOnFirstDivergence: true,
		api.StopOnFirstCrash:      true,
		api.StopOnNone:            true,
	}
	for i, v := range s.Budget.StopOn {
		if !allowedStopOn[v] {
			add(fmt.Sprintf("budget.stop_on[%d]", i), fmt.Sprintf("unknown stop_on value %q", v), "one of: first_divergence, first_crash, none")
		}
	}

	allowedOracle := map[string]bool{
		api.OracleStateDiff:         true,
		api.OracleConsensusLiveness: true,
		api.OraclePeerHealth:        true,
	}
	for i, v := range s.Oracles {
		if !allowedOracle[v] {
			add(fmt.Sprintf("oracles[%d]", i), fmt.Sprintf("unknown oracle %q", v), "one of: state_diff, consensus_liveness, peer_health")
		}
	}

	if s.Services != nil {
		validateImage("services.sidecar_image", s.Services.SidecarImage, s.Services.SidecarImage != "", add)
		if s.Workload.Kind != api.WorkloadNone && s.Services.Control != nil && !*s.Services.Control {
			add("services.control", "services.control must be enabled for workload runs", "omit services.control or set it to true; only workload.kind=none may disable control")
		}
	}

	return errs
}

func validateNodeGroup(field string, group api.NodeGroup, add func(string, string, string)) {
	validateImage(field+".image", group.Image, group.Image != "", add)
	if group.Nodes != nil && len(group.Nodes) != group.Count {
		add(field+".nodes", fmt.Sprintf("nodes must contain exactly count entries (got %d, count %d)", len(group.Nodes), group.Count), "provide one node override for every rippled node")
	}
	validateEntrypoint(field+".entrypoint", group.Entrypoint, add)
	validateConfig(field+".config", group.Config, add)
	for i, node := range group.Nodes {
		nodeField := fmt.Sprintf("%s.nodes[%d]", field, i)
		if node.Image != nil {
			validateImage(nodeField+".image", *node.Image, true, add)
		}
		validateEntrypoint(nodeField+".entrypoint", node.Entrypoint, add)
		validateConfig(nodeField+".config", node.Config, add)
	}
}

func validateImage(field, image string, supplied bool, add func(string, string, string)) {
	if !supplied {
		return
	}
	trimmed := strings.TrimSpace(image)
	if trimmed == "" {
		add(field, "image must be non-empty; whitespace-only values are invalid", "omit the image to use the Starlark default or provide a valid image reference")
		return
	}
	if trimmed != image {
		add(field, "image must not contain leading or trailing whitespace", "use a valid image reference")
	}
}

func validateEntrypoint(field string, entrypoint []string, add func(string, string, string)) {
	if entrypoint == nil {
		return
	}
	if len(entrypoint) == 0 {
		add(field, "entrypoint must contain at least one command", "omit entrypoint or provide a non-empty command list")
		return
	}
	for i, command := range entrypoint {
		if strings.TrimSpace(command) == "" {
			add(fmt.Sprintf("%s[%d]", field, i), "entrypoint command must be non-empty", "provide an executable or argument")
		}
	}
}

func validateConfig(field string, config map[string][]string, add func(string, string, string)) {
	for section, lines := range config {
		sectionField := field + "." + section
		if strings.TrimSpace(section) == "" {
			add(sectionField, "config section name must be non-empty", "use an INI section name")
			continue
		}
		if strings.TrimSpace(section) != section || strings.ContainsAny(section, "\r\n[]") {
			add(sectionField, "config section name contains invalid whitespace, newline, or section delimiters", "use a plain INI section name without brackets")
		}
		if _, guarded := guardedConfigSections[strings.ToLower(section)]; guarded {
			add(sectionField, fmt.Sprintf("config section %q is reserved; use the dedicated scenario field", section), "set network.network_id, network.amendments, or network.veto_amendments where applicable")
		}
		if lines == nil {
			add(sectionField, "config section entries must be a list, not null", "use [] for an explicitly empty section")
			continue
		}
		for i, line := range lines {
			lineField := fmt.Sprintf("%s[%d]", sectionField, i)
			if strings.ContainsAny(line, "\r\n") {
				add(lineField, "config entries must be single-line values", "split multiline configuration into separate list entries")
				continue
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") {
				add(lineField, "config entries must not inject an INI section", "put each section under its own config key")
			}
		}
	}
}

func validateAmendments(field string, amendments *[]api.Amendment, add func(string, string, string)) {
	if amendments == nil {
		return
	}
	seen := make(map[string]int, len(*amendments))
	for i, amendment := range *amendments {
		entryField := fmt.Sprintf("%s[%d]", field, i)
		if !amendmentIDRE.MatchString(amendment.ID) {
			add(entryField+".id", fmt.Sprintf("amendment id must be exactly 64 hexadecimal characters (got %q)", amendment.ID), "use the amendment's 64-character SHA-512-half ID")
		}
		if !amendmentNameRE.MatchString(amendment.Name) {
			add(entryField+".name", fmt.Sprintf("amendment name must contain only letters, digits, and underscores (got %q)", amendment.Name), "use the rippled amendment name")
		}
		key := strings.ToUpper(amendment.ID)
		if prior, ok := seen[key]; ok {
			add(entryField+".id", fmt.Sprintf("amendment id duplicates %s[%d]", field, prior), "list each amendment ID once")
		} else {
			seen[key] = i
		}
	}
}

func validateAmendmentOverlap(upvotes, vetoes *[]api.Amendment, add func(string, string, string)) {
	if upvotes == nil || vetoes == nil {
		return
	}
	upvoteIDs := make(map[string]int, len(*upvotes))
	for i, amendment := range *upvotes {
		upvoteIDs[strings.ToUpper(amendment.ID)] = i
	}
	for i, amendment := range *vetoes {
		if prior, ok := upvoteIDs[strings.ToUpper(amendment.ID)]; ok {
			add(fmt.Sprintf("network.veto_amendments[%d].id", i), fmt.Sprintf("amendment id also appears in network.amendments[%d]", prior), "an amendment cannot be both upvoted and vetoed")
		}
	}
}
