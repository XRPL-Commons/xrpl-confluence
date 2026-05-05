# Testnet Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the next four follow-ups for the long-lived testnet — dashboard verification, periodic corpus extraction, multi-tier account pool, xrpl-go-signed `tx_blob` submission, and chaos-event targeting against goXRPL containers.

**Architecture:** Four independent phases, each shippable on its own. Phase G is operational polish (dashboard panel + a corpus-extraction cron). Phase H replaces `accounts.RotateTiers`'s no-op with a real tier classification: rich, at-reserve, multisig, regular-key, blackholed accounts. Phase I shifts tx submission from rippled-side `sign_and_submit` (secret on the wire) to xrpl-go-signed `tx_blob` (locally signed). Phase J builds a `goxrpl-tools:latest` image that combines goXRPL with iproute2/iptables so chaos events can exercise goXRPL's network-fault recovery.

**Tech Stack:** Go 1.24, `github.com/Peersyst/xrpl-go/xrpl/wallet` (already vendored), `github.com/docker/docker/client` (already vendored), Kurtosis 1.16 Starlark, Docker multi-stage builds for `goxrpl-tools`. No new Go dependencies.

**Repository roots referenced throughout:**
- `xrpl-confluence/` — working dir for all `git`/`kurtosis`/`make`/`go`/`docker` commands.
- `xrpl-confluence/sidecar/internal/fuzz/{accounts,generator,runners}/` — existing Go packages we modify.
- `xrpl-confluence/sidecar/internal/rpcclient/` — JSON-RPC client we extend.
- `xrpl-confluence/src/{tests,sidecar,goxrpl,helpers}/` — Starlark.
- `xrpl-confluence/dashboard/` — Node 22 dashboard.
- `xrpl-confluence/scripts/` — bash build scripts.

**File Structure:**

Phase G:
- `dashboard/static/app.js` — **modify.** Sort `divergences_total_by_layer` rows; ensure `chaos` row renders.
- `Makefile` — **modify.** Add `soak-pull-loop`/`chaos-pull-loop` targets and a shared `_pull-loop` helper.
- `scripts/corpus-pull-loop.sh` — **new.** Tiny bash loop calling `docker cp` every N seconds.

Phase H:
- `sidecar/internal/fuzz/accounts/tiers.go` — **new.** `Tier` enum + per-tier setup helpers + `AssignTiers(pool, weights)`.
- `sidecar/internal/fuzz/accounts/tiers_test.go` — **new.** Unit tests for tier assignment, deterministic weight sampling, setup helpers (mocked rpcclient).
- `sidecar/internal/fuzz/accounts/pool.go` — **modify.** `Wallet` gains a `Tier` field; `Pool.PickTier(tier, rng)` helper; `RotateTiers` becomes a real top-up loop for at-reserve drift.
- `sidecar/internal/fuzz/accounts/setup.go` — **modify.** After the existing `SetupState`, call `tiers.ApplyAll(submit, pool)` to perform per-tier account configuration (SignerListSet, SetRegularKey, AccountSet asfDisableMaster).
- `sidecar/internal/fuzz/generator/tracker.go` — **modify.** Optional: tier-aware tx selection (pick from blackholed pool when generating intentionally-failing tx, etc.). Minimal additions only — most generators just pick from `Pool.All()` as today.

Phase I:
- `sidecar/internal/rpcclient/sign.go` — **new.** `(*Client).SubmitTxBlob(blob)` + a `Sign(secret, tx, autofillCtx)` helper that fetches account_info / fee / ledger_current and produces a fully-formed signed `tx_blob` via xrpl-go's `wallet.Sign`.
- `sidecar/internal/rpcclient/sign_test.go` — **new.** httptest-mocked tests covering Sign autofill paths and SubmitTxBlob round-trip.
- `sidecar/internal/rpcclient/client.go` — **modify.** Add `SubmitTxBlob(blob)` method; existing `SubmitTxJSON` left in place behind a `LocalSign bool` flag on the runner config.
- `sidecar/internal/fuzz/runners/realtime.go` — **modify.** When `cfg.LocalSign == true`, route through the new sign-then-submit-blob path.
- `sidecar/internal/fuzz/runners/soak.go` — **modify.** Same.
- `sidecar/cmd/fuzz/main.go` — **modify.** Read `LOCAL_SIGN` env var (default false for now).

Phase J:
- `scripts/build-goxrpl-tools.sh` — **new.** Multi-stage `docker build` producing `goxrpl-tools:latest` (Debian slim + iproute2 + iptables + COPY --from=goxrpl:latest the binary).
- `goxrpl-tools.Dockerfile` — **new.** The Dockerfile invoked by `scripts/build-goxrpl-tools.sh`.
- `src/goxrpl/goxrpl.star` — **modify.** Accept an `enable_chaos_tools=False` arg; when true, switch the image from `goxrpl:latest` to `goxrpl-tools:latest`.
- `main.star` — **modify.** Pass `enable_chaos_tools=True` from the chaos suite path; default false elsewhere.
- `src/tests/chaos.star` — **modify.** Set `enable_chaos_tools=True` somewhere or document that chaos suite needs the tools image.

---

## Phase G — Operational polish

### Task G1: Verify chaos layer renders + sort divergences table

**Files:**
- Modify: `dashboard/static/app.js`

The dashboard already iterates `divergences_total_by_layer` (`app.js:464`), so a `layer="chaos"` counter from E1's metrics surface will appear. Visual polish: sort rows for stable ordering (current code uses Object.entries iteration order, which is insertion order — surprising when layers appear at different times).

- [ ] **Step 1: Read the existing `pollFuzz()` body**

```bash
sed -n '440,475p' dashboard/static/app.js
```

Confirm the `Object.entries(data.divergences_total_by_layer ?? {})` loop at the location grep flagged earlier (`app.js:464`).

- [ ] **Step 2: Sort the entries by layer name**

Edit `dashboard/static/app.js`. Replace:

```js
for (const [layer, count] of Object.entries(data.divergences_total_by_layer ?? {})) {
```

with:

```js
const layerEntries = Object.entries(data.divergences_total_by_layer ?? {})
  .sort(([a], [b]) => a.localeCompare(b));
for (const [layer, count] of layerEntries) {
```

This keeps the rendered order deterministic (`chaos`, `crash`, `invariant`, `metadata`, `state_hash`, `tx_result` alphabetical).

- [ ] **Step 3: Smoke-verify rendering with a dummy chaos counter**

Without standing up a full enclave, hit the dashboard's `/api/fuzz` directly with a stub. Skip this — the proper visual smoke runs in Task J4 once chaos events fire end-to-end. A pure JS sort change is mechanically obvious.

- [ ] **Step 4: Commit**

```bash
git add dashboard/static/app.js
git commit -m "dashboard: sort divergences-by-layer rows for stable ordering"
```

Commit message body must be empty. Never mention "Claude", "AI", or any agent.

### Task G2: Periodic corpus extraction

**Files:**
- Create: `scripts/corpus-pull-loop.sh`
- Modify: `Makefile`

The current `make soak-pull` / `make chaos-pull` targets do a one-shot `docker cp`. For week-long soaks the operator needs a tail-style loop. Simplest approach: a bash script that wraps `docker cp` in a `while sleep N` loop and writes to a host directory.

- [ ] **Step 1: Create the loop script**

```bash
mkdir -p scripts
```

`scripts/corpus-pull-loop.sh`:

```bash
#!/usr/bin/env bash
# Periodically extract /output/corpus from a Kurtosis-managed sidecar to a
# host path. Designed for week-long soak runs where leaving "make soak-pull"
# in a manual loop is brittle.
#
# Usage: corpus-pull-loop.sh <enclave> <service> <interval-seconds> <host-dir>
set -euo pipefail

if [[ $# -ne 4 ]]; then
	echo "usage: $0 <enclave> <service> <interval-seconds> <host-dir>" >&2
	exit 2
fi

ENCLAVE="$1"
SERVICE="$2"
INTERVAL="$3"
HOSTDIR="$4"

mkdir -p "$HOSTDIR"

while true; do
	UUID=$(kurtosis service inspect "$ENCLAVE" "$SERVICE" 2>/dev/null \
		| awk '/^UUID:/ {print $2; exit}' || true)
	if [[ -n "$UUID" ]]; then
		CONTAINER=$(docker ps --format '{{.Names}}' | grep "^$SERVICE--$UUID" | head -1 || true)
		if [[ -n "$CONTAINER" ]]; then
			docker cp "$CONTAINER:/output/corpus" "$HOSTDIR/" 2>/dev/null \
				&& echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] pulled $SERVICE corpus to $HOSTDIR/" \
				|| echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $SERVICE corpus pull skipped (transient)" >&2
		fi
	fi
	sleep "$INTERVAL"
done
```

```bash
chmod +x scripts/corpus-pull-loop.sh
```

- [ ] **Step 2: Add `*-pull-loop` Makefile targets**

Edit `Makefile`. Find the `.PHONY` line and add `soak-pull-loop chaos-pull-loop` to it. Append two new targets at the end of the file:

```make
PULL_INTERVAL ?= 300

soak-pull-loop:
	bash scripts/corpus-pull-loop.sh $(ENCLAVE) fuzz-soak $(PULL_INTERVAL) $(CORPUS)

chaos-pull-loop:
	bash scripts/corpus-pull-loop.sh $(CHAOS_ENCLAVE) fuzz-chaos $(PULL_INTERVAL) $(CHAOS_CORPUS)
```

- [ ] **Step 3: Validate the script syntax**

```bash
bash -n scripts/corpus-pull-loop.sh
make -n soak-pull-loop
make -n chaos-pull-loop
```

Expected: no syntax errors; `make -n` expands to a `bash scripts/corpus-pull-loop.sh xrpl-soak fuzz-soak 300 ...` line.

- [ ] **Step 4: Commit**

```bash
git add scripts/corpus-pull-loop.sh Makefile
git commit -m "confluence: periodic corpus extraction (make soak-pull-loop / chaos-pull-loop)"
```

---

## Phase H — Multi-tier account pool

The current pool is rich-only — every wallet has plenty of XRP and a healthy master key. To exercise reserve-boundary code, signer-list logic, regular-key paths, and blackholed-account behavior, we tier the pool. Each tier is configured once during `SetupState`'s post-setup step; tx generation continues to draw from `Pool.All()` (no behavior change required for existing generators), but new tier-aware generators can opt in via `Pool.PickTier`.

### Task H1: Define `Tier` enum + `Wallet.Tier` field

**Files:**
- Create: `sidecar/internal/fuzz/accounts/tiers.go`
- Create: `sidecar/internal/fuzz/accounts/tiers_test.go`
- Modify: `sidecar/internal/fuzz/accounts/keys.go` — add `Tier` field to `Wallet`.
- Modify: `sidecar/internal/fuzz/accounts/pool.go` — `Pool.PickTier(tier, rng)` helper.

- [ ] **Step 1: Failing test**

`sidecar/internal/fuzz/accounts/tiers_test.go`:

```go
package accounts

import (
	"math/rand/v2"
	"testing"
)

func TestAssignTiers_SpreadsAcrossTiers(t *testing.T) {
	pool, err := NewPool(42, 20)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{
		Rich:       10,
		AtReserve:  4,
		Multisig:   3,
		RegularKey: 2,
		Blackholed: 1,
	}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)

	counts := map[Tier]int{}
	for _, w := range pool.All() {
		counts[w.Tier]++
	}
	if counts[Rich]+counts[AtReserve]+counts[Multisig]+counts[RegularKey]+counts[Blackholed] != 20 {
		t.Errorf("counts = %+v, want sum 20", counts)
	}
	if counts[Rich] < 5 {
		t.Errorf("Rich count = %d, want >= 5 (weight 10/20)", counts[Rich])
	}
	if counts[Blackholed] < 1 {
		t.Errorf("Blackholed count = %d, want >= 1", counts[Blackholed])
	}
}

func TestPickTier_ReturnsCorrectTier(t *testing.T) {
	pool, err := NewPool(42, 10)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 5, AtReserve: 3, Multisig: 2}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)

	for i := 0; i < 20; i++ {
		w := pool.PickTier(Multisig, rng)
		if w == nil {
			t.Fatal("PickTier(Multisig) returned nil")
		}
		if w.Tier != Multisig {
			t.Errorf("Tier = %v, want Multisig", w.Tier)
		}
	}
}

func TestPickTier_NilWhenEmpty(t *testing.T) {
	pool, err := NewPool(42, 5)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 5}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)
	if w := pool.PickTier(Blackholed, rng); w != nil {
		t.Errorf("PickTier(Blackholed) = %+v, want nil (no blackholed wallets)", w)
	}
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
cd sidecar
go test ./internal/fuzz/accounts/...
```

Expected: errors citing `Tier`, `TierWeights`, `AssignTiers`, `Pool.PickTier`, and `Wallet.Tier` undefined.

- [ ] **Step 3: Add `Tier` field to `Wallet`**

Edit `sidecar/internal/fuzz/accounts/keys.go`. Find:

```go
type Wallet struct {
	Index          int
	ClassicAddress string
	Seed           string
}
```

Replace with:

```go
type Wallet struct {
	Index          int
	ClassicAddress string
	Seed           string
	Tier           Tier
}
```

(The `Tier` type is defined in tiers.go — Step 4. The build will fail until both files are written.)

- [ ] **Step 4: Implement tiers.go**

`sidecar/internal/fuzz/accounts/tiers.go`:

```go
package accounts

import (
	"math/rand/v2"
)

// Tier classifies a pool wallet by its account-state shape. Tier-specific
// setup runs once during SetupState (see ApplyAll); tier-aware generators
// can use Pool.PickTier to draw a wallet of a specific class.
type Tier int

const (
	// Rich: well above reserve, master key enabled, no signer list, no
	// regular key. Default for the M1 pool. Most generators target rich
	// accounts.
	Rich Tier = iota
	// AtReserve: balance trimmed to exactly the reserve_base. New tx that
	// would push the account below reserve must fail with tecINSUFF_RESERVE.
	AtReserve
	// Multisig: signer list installed (3 signers, quorum 2), master key
	// still enabled. Multisigned tx exercise SignerListSet / multisign paths.
	Multisig
	// RegularKey: a regular key set, master key disabled (asfDisableMaster).
	// Tx must be signed by the regular key, not the master.
	RegularKey
	// Blackholed: master key disabled, no regular key set. The account is
	// fully inert — every tx submitted from it must fail. Useful for testing
	// the failure-mode codepath (tefMASTER_DISABLED + tefNO_AUTH_REQUIRED).
	Blackholed
)

func (t Tier) String() string {
	switch t {
	case Rich:
		return "Rich"
	case AtReserve:
		return "AtReserve"
	case Multisig:
		return "Multisig"
	case RegularKey:
		return "RegularKey"
	case Blackholed:
		return "Blackholed"
	default:
		return "Unknown"
	}
}

// TierWeights expresses the desired distribution of wallets across tiers.
// Each field is a non-negative integer; the pool is partitioned proportionally.
// Weights of zero produce zero wallets in that tier.
type TierWeights struct {
	Rich       int
	AtReserve  int
	Multisig   int
	RegularKey int
	Blackholed int
}

// AssignTiers stamps each wallet in the pool with a Tier according to the
// weight distribution. Deterministic for a given (rng, weights). After the
// call, Pool.PickTier returns wallets from the requested tier.
func AssignTiers(pool *Pool, weights TierWeights, rng *rand.Rand) {
	wallets := pool.All()
	total := weights.Rich + weights.AtReserve + weights.Multisig + weights.RegularKey + weights.Blackholed
	if total == 0 {
		// Default: all rich.
		for _, w := range wallets {
			w.Tier = Rich
		}
		return
	}

	// Compute target counts: floor(weight/total * len), then distribute the
	// remainder to Rich (the safe default).
	targets := map[Tier]int{
		Rich:       weights.Rich * len(wallets) / total,
		AtReserve:  weights.AtReserve * len(wallets) / total,
		Multisig:   weights.Multisig * len(wallets) / total,
		RegularKey: weights.RegularKey * len(wallets) / total,
		Blackholed: weights.Blackholed * len(wallets) / total,
	}
	assigned := targets[Rich] + targets[AtReserve] + targets[Multisig] + targets[RegularKey] + targets[Blackholed]
	targets[Rich] += len(wallets) - assigned

	// Stamp wallets in order: indices 0..N-1 get tier T while targets[T] > 0.
	tiers := []Tier{Rich, AtReserve, Multisig, RegularKey, Blackholed}
	idx := 0
	for _, t := range tiers {
		for k := 0; k < targets[t]; k++ {
			wallets[idx].Tier = t
			idx++
		}
	}
	_ = rng // reserved for future shuffle; deterministic order is fine for now.
}
```

- [ ] **Step 5: Add `Pool.PickTier`**

Edit `sidecar/internal/fuzz/accounts/pool.go`. Append:

```go
// PickTier returns a uniformly random wallet of the requested tier, or nil
// if no wallet in the pool matches.
func (p *Pool) PickTier(t Tier, r *mathrand.Rand) *Wallet {
	matching := []*Wallet{}
	for _, w := range p.wallets {
		if w.Tier == t {
			matching = append(matching, w)
		}
	}
	if len(matching) == 0 {
		return nil
	}
	return matching[r.IntN(len(matching))]
}
```

- [ ] **Step 6: Run — expect PASS**

```bash
go test ./internal/fuzz/accounts/...
```

- [ ] **Step 7: Commit**

```bash
git add sidecar/internal/fuzz/accounts/tiers.go sidecar/internal/fuzz/accounts/tiers_test.go sidecar/internal/fuzz/accounts/keys.go sidecar/internal/fuzz/accounts/pool.go
git commit -m "accounts: Tier enum + AssignTiers + Pool.PickTier"
```

### Task H2: Tier setup — AtReserve

**Files:**
- Modify: `sidecar/internal/fuzz/accounts/tiers.go` — add `setupAtReserve(submit, w)` helper.
- Modify: `sidecar/internal/fuzz/accounts/tiers_test.go` — add `TestSetupAtReserve_SubmitsPaymentToTreasury`.

The "at-reserve" tier is achieved by paying out *most* of the wallet's balance to the genesis (treasury), leaving exactly `reserve_base` (200_000_000 drops) plus the next tx's fee buffer.

- [ ] **Step 1: Failing test**

Append to `sidecar/internal/fuzz/accounts/tiers_test.go`:

```go
import (
	// ... existing imports
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// stubSubmit captures every SubmitTxJSON call to verify intent.
type stubSubmit struct {
	calls []map[string]any
}

func (s *stubSubmit) SubmitTxJSON(secret string, tx map[string]any) (*rpcclient.SubmitResult, error) {
	s.calls = append(s.calls, tx)
	return &rpcclient.SubmitResult{EngineResult: "tesSUCCESS"}, nil
}

// AccountInfo stub used by the at-reserve setup to fetch current balance.
func (s *stubSubmit) AccountInfo(addr string) (*rpcclient.AccountInfoResult, error) {
	return &rpcclient.AccountInfoResult{
		Account:  addr,
		Balance:  "10000000000", // 10000 XRP, well above reserve
		Sequence: 1,
	}, nil
}

func TestSetupAtReserve_SubmitsPaymentToTreasury(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest1", Seed: "sTest1", Tier: AtReserve}
	stub := &stubSubmit{}
	if err := setupAtReserve(stub, w); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("submit calls = %d, want 1", len(stub.calls))
	}
	tx := stub.calls[0]
	if tx["TransactionType"] != "Payment" {
		t.Errorf("tx_type = %v, want Payment", tx["TransactionType"])
	}
	if tx["Destination"] != GenesisAddress {
		t.Errorf("destination = %v, want %s", tx["Destination"], GenesisAddress)
	}
}
```

The test references `GenesisAddress` (already exported from the accounts package via `funding.go`) and a `submitter` interface that we'll define in tiers.go. The stub above intentionally satisfies a minimal interface; we'll define it inline in tiers.go.

- [ ] **Step 2: Run — expect build failure**

```bash
go test ./internal/fuzz/accounts/...
```

- [ ] **Step 3: Implement `setupAtReserve`**

Add to `sidecar/internal/fuzz/accounts/tiers.go`:

```go
import (
	// existing imports
	"fmt"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/rpcclient"
)

// reserveBaseDrops is the XRPL reserve_base (200 XRP). Hard-coded — matches
// the test network's reserve setting in topology.star.
const reserveBaseDrops = 200_000_000

// submitter is the minimal interface tier setup needs: tx submission +
// account_info lookup. The full *rpcclient.Client satisfies it; tests inject
// a stub.
type submitter interface {
	SubmitTxJSON(secret string, tx map[string]any) (*rpcclient.SubmitResult, error)
	AccountInfo(addr string) (*rpcclient.AccountInfoResult, error)
}

// setupAtReserve drains the wallet's balance to exactly reserve_base by
// sending the excess to the genesis address. After this, any tx that
// requires sending more drops than (balance - reserve - fee) must fail
// with tecINSUFF_RESERVE — exactly the boundary we want to exercise.
func setupAtReserve(s submitter, w *Wallet) error {
	info, err := s.AccountInfo(w.ClassicAddress)
	if err != nil {
		return fmt.Errorf("at-reserve: account_info %s: %w", w.ClassicAddress, err)
	}
	balance, err := parseDrops(info.Balance)
	if err != nil {
		return fmt.Errorf("at-reserve: parse balance %q: %w", info.Balance, err)
	}
	// Send everything except reserve_base + 100 drops fee buffer.
	excess := balance - reserveBaseDrops - 100
	if excess <= 0 {
		return nil // already at or below reserve; nothing to do
	}
	_, err = s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "Payment",
		"Account":         w.ClassicAddress,
		"Destination":     GenesisAddress,
		"Amount":          fmt.Sprintf("%d", excess),
	})
	return err
}

// parseDrops parses an XRPL balance string (decimal drops as string) into int64.
func parseDrops(s string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/accounts/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/accounts/tiers.go sidecar/internal/fuzz/accounts/tiers_test.go
git commit -m "accounts: AtReserve tier setup (drain to reserve_base)"
```

### Task H3: Tier setup — Multisig

**Files:**
- Modify: `sidecar/internal/fuzz/accounts/tiers.go` — `setupMultisig(submit, w, signers)`.
- Modify: `sidecar/internal/fuzz/accounts/tiers_test.go` — `TestSetupMultisig_InstallsSignerList`.

A Multisig account installs a SignerListSet: 3 signers, quorum 2. The signer pubkeys come from sibling pool wallets so they're deterministic and we don't need fresh keys.

- [ ] **Step 1: Failing test**

Append to `tiers_test.go`:

```go
func TestSetupMultisig_InstallsSignerList(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest", Seed: "sTest", Tier: Multisig}
	signers := []*Wallet{
		{Index: 1, ClassicAddress: "rA", Seed: "sA"},
		{Index: 2, ClassicAddress: "rB", Seed: "sB"},
		{Index: 3, ClassicAddress: "rC", Seed: "sC"},
	}
	stub := &stubSubmit{}
	if err := setupMultisig(stub, w, signers); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("submit calls = %d, want 1", len(stub.calls))
	}
	tx := stub.calls[0]
	if tx["TransactionType"] != "SignerListSet" {
		t.Errorf("tx_type = %v, want SignerListSet", tx["TransactionType"])
	}
	if tx["SignerQuorum"] != uint32(2) {
		t.Errorf("quorum = %v, want 2", tx["SignerQuorum"])
	}
	entries, ok := tx["SignerEntries"].([]map[string]any)
	if !ok || len(entries) != 3 {
		t.Errorf("entries = %+v, want 3", tx["SignerEntries"])
	}
}
```

- [ ] **Step 2: Run — expect build failure (`setupMultisig` undefined)**

- [ ] **Step 3: Implement `setupMultisig`**

Add to `tiers.go`:

```go
// setupMultisig installs a 3-of-quorum-2 SignerListSet on w using the given
// signer wallets' classic addresses. Master key remains enabled so the wallet
// can still sign single-sig — multisig is in addition, not replacement.
func setupMultisig(s submitter, w *Wallet, signers []*Wallet) error {
	if len(signers) < 3 {
		return fmt.Errorf("multisig: need >= 3 signers, got %d", len(signers))
	}
	entries := make([]map[string]any, 0, 3)
	for _, sg := range signers[:3] {
		entries = append(entries, map[string]any{
			"SignerEntry": map[string]any{
				"Account":      sg.ClassicAddress,
				"SignerWeight": uint32(1),
			},
		})
	}
	_, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "SignerListSet",
		"Account":         w.ClassicAddress,
		"SignerQuorum":    uint32(2),
		"SignerEntries":   entries,
	})
	return err
}
```

NOTE: The XRPL wire format wraps each entry in a `SignerEntry` object. The test asserts `len(entries) == 3` against `tx["SignerEntries"]` directly — confirm the assertion matches the actual structure your impl uses. If it doesn't, adjust the test's `entries, ok := tx["SignerEntries"].([]map[string]any)` parse to extract the unwrapped entries first.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/accounts/...
```

If the test fails because of the SignerEntry wrapping shape, fix the test's parse (NOT the production code — XRPL's wire format requires the wrap).

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/accounts/tiers.go sidecar/internal/fuzz/accounts/tiers_test.go
git commit -m "accounts: Multisig tier setup (SignerListSet 3-signers quorum-2)"
```

### Task H4: Tier setup — RegularKey

**Files:**
- Modify: `sidecar/internal/fuzz/accounts/tiers.go` — `setupRegularKey(submit, w, regKeyAddr)`.
- Modify: `sidecar/internal/fuzz/accounts/tiers_test.go` — `TestSetupRegularKey_SetsKeyThenDisablesMaster`.

A RegularKey wallet sets a regular key first (so the account isn't locked), then disables the master key via `AccountSet asfDisableMaster` (flag value 4).

- [ ] **Step 1: Failing test**

Append to `tiers_test.go`:

```go
func TestSetupRegularKey_SetsKeyThenDisablesMaster(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest", Seed: "sTest", Tier: RegularKey}
	stub := &stubSubmit{}
	if err := setupRegularKey(stub, w, "rRegKey"); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 {
		t.Fatalf("submit calls = %d, want 2 (SetRegularKey + AccountSet)", len(stub.calls))
	}
	if stub.calls[0]["TransactionType"] != "SetRegularKey" {
		t.Errorf("call[0] = %v, want SetRegularKey", stub.calls[0]["TransactionType"])
	}
	if stub.calls[0]["RegularKey"] != "rRegKey" {
		t.Errorf("RegularKey = %v, want rRegKey", stub.calls[0]["RegularKey"])
	}
	if stub.calls[1]["TransactionType"] != "AccountSet" {
		t.Errorf("call[1] = %v, want AccountSet", stub.calls[1]["TransactionType"])
	}
	if stub.calls[1]["SetFlag"] != uint32(4) {
		t.Errorf("SetFlag = %v, want 4 (asfDisableMaster)", stub.calls[1]["SetFlag"])
	}
}
```

- [ ] **Step 2: Run — expect build failure**

- [ ] **Step 3: Implement `setupRegularKey`**

Add to `tiers.go`:

```go
// asfDisableMaster is the AccountSet flag that disables the master keypair.
// After it lands, the master seed can no longer sign tx — regular key only.
const asfDisableMaster = uint32(4)

// setupRegularKey installs a RegularKey then disables the master. ORDER
// MATTERS: disabling the master before installing a regular key would lock
// the account out forever.
func setupRegularKey(s submitter, w *Wallet, regKeyAddr string) error {
	if _, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "SetRegularKey",
		"Account":         w.ClassicAddress,
		"RegularKey":      regKeyAddr,
	}); err != nil {
		return fmt.Errorf("regkey: SetRegularKey %s: %w", w.ClassicAddress, err)
	}
	if _, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "AccountSet",
		"Account":         w.ClassicAddress,
		"SetFlag":         asfDisableMaster,
	}); err != nil {
		return fmt.Errorf("regkey: disable-master %s: %w", w.ClassicAddress, err)
	}
	return nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/accounts/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/accounts/tiers.go sidecar/internal/fuzz/accounts/tiers_test.go
git commit -m "accounts: RegularKey tier setup (SetRegularKey then asfDisableMaster)"
```

### Task H5: Tier setup — Blackholed + ApplyAll dispatcher

**Files:**
- Modify: `sidecar/internal/fuzz/accounts/tiers.go` — `setupBlackholed(submit, w)` + `ApplyAll(submit, pool)`.
- Modify: `sidecar/internal/fuzz/accounts/tiers_test.go` — `TestSetupBlackholed_DisablesMaster` + `TestApplyAll_RoutesByTier`.

Blackholed = master disabled with no regular key set (just the AccountSet flag — no SetRegularKey first). The account becomes inert; every tx submitted from it MUST fail.

`ApplyAll` is the orchestrator that walks the pool and calls the right setup function per Tier, skipping Rich (no setup needed).

- [ ] **Step 1: Failing tests**

Append to `tiers_test.go`:

```go
func TestSetupBlackholed_DisablesMaster(t *testing.T) {
	w := &Wallet{Index: 0, ClassicAddress: "rTest", Seed: "sTest", Tier: Blackholed}
	stub := &stubSubmit{}
	if err := setupBlackholed(stub, w); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("submit calls = %d, want 1", len(stub.calls))
	}
	if stub.calls[0]["TransactionType"] != "AccountSet" {
		t.Errorf("tx_type = %v", stub.calls[0]["TransactionType"])
	}
	if stub.calls[0]["SetFlag"] != uint32(4) {
		t.Errorf("SetFlag = %v, want 4 (asfDisableMaster)", stub.calls[0]["SetFlag"])
	}
}

func TestApplyAll_RoutesByTier(t *testing.T) {
	pool, err := NewPool(42, 10)
	if err != nil {
		t.Fatal(err)
	}
	weights := TierWeights{Rich: 4, AtReserve: 2, Multisig: 2, RegularKey: 1, Blackholed: 1}
	rng := rand.New(rand.NewPCG(42, 0))
	AssignTiers(pool, weights, rng)
	stub := &stubSubmit{}
	if err := ApplyAll(stub, pool); err != nil {
		t.Fatal(err)
	}
	// Rich = 0 calls. AtReserve = 1 call. Multisig = 1. RegularKey = 2. Blackholed = 1.
	// Total: 0*4 + 1*2 + 1*2 + 2*1 + 1*1 = 7.
	if len(stub.calls) != 7 {
		t.Fatalf("submit calls = %d, want 7", len(stub.calls))
	}
}
```

- [ ] **Step 2: Run — expect build failure**

- [ ] **Step 3: Implement `setupBlackholed` + `ApplyAll`**

Add to `tiers.go`:

```go
// setupBlackholed disables the master without setting a regular key. The
// account is now inert. Every subsequent tx from it must fail with
// tefMASTER_DISABLED (or similar).
func setupBlackholed(s submitter, w *Wallet) error {
	_, err := s.SubmitTxJSON(w.Seed, map[string]any{
		"TransactionType": "AccountSet",
		"Account":         w.ClassicAddress,
		"SetFlag":         asfDisableMaster,
	})
	return err
}

// ApplyAll walks the pool and runs the appropriate per-tier setup on each
// wallet. Rich tier is a no-op (default state). Errors short-circuit and
// are returned with the failing wallet's address attached.
func ApplyAll(s submitter, pool *Pool) error {
	wallets := pool.All()
	for _, w := range wallets {
		var err error
		switch w.Tier {
		case Rich:
			continue
		case AtReserve:
			err = setupAtReserve(s, w)
		case Multisig:
			signers := otherWallets(wallets, w, 3)
			if len(signers) < 3 {
				return fmt.Errorf("multisig %s: pool has < 3 other wallets", w.ClassicAddress)
			}
			err = setupMultisig(s, w, signers)
		case RegularKey:
			signers := otherWallets(wallets, w, 1)
			if len(signers) < 1 {
				return fmt.Errorf("regkey %s: pool has < 1 other wallet", w.ClassicAddress)
			}
			err = setupRegularKey(s, w, signers[0].ClassicAddress)
		case Blackholed:
			err = setupBlackholed(s, w)
		}
		if err != nil {
			return fmt.Errorf("tier %s on %s: %w", w.Tier, w.ClassicAddress, err)
		}
	}
	return nil
}

// otherWallets returns up to n wallets from the pool whose ClassicAddress
// is not equal to skip's. Used to draw signers/regular-keys from siblings.
func otherWallets(wallets []*Wallet, skip *Wallet, n int) []*Wallet {
	out := make([]*Wallet, 0, n)
	for _, w := range wallets {
		if w.ClassicAddress == skip.ClassicAddress {
			continue
		}
		out = append(out, w)
		if len(out) == n {
			return out
		}
	}
	return out
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/fuzz/accounts/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/accounts/tiers.go sidecar/internal/fuzz/accounts/tiers_test.go
git commit -m "accounts: Blackholed tier + ApplyAll dispatcher"
```

### Task H6: Wire ApplyAll + replace stub RotateTiers

**Files:**
- Modify: `sidecar/internal/fuzz/accounts/setup.go` — call `ApplyAll` after the existing two-phase setup.
- Modify: `sidecar/internal/fuzz/accounts/pool.go` — `RotateTiers` performs a real refill on at-reserve wallets that have drifted.
- Modify: `sidecar/internal/fuzz/runners/realtime.go` and `soak.go` — call `accounts.AssignTiers(pool, weights, rng.Rand())` after pool construction; expose tier weights via Config.

- [ ] **Step 1: Add `TierWeights` to runner Config**

Edit `sidecar/internal/fuzz/runners/realtime.go`. Add to `Config`:

```go
// TierWeights configures the multi-tier account pool. Zero-value means
// rich-only (preserves M1 behavior). Set non-zero AtReserve / Multisig /
// RegularKey / Blackholed to exercise tier-specific code paths.
TierWeights accounts.TierWeights
```

The `accounts` import is likely already present in realtime.go; confirm.

- [ ] **Step 2: Wire `AssignTiers` into Run + SoakRun**

In both `realtime.go::Run` and `soak.go::SoakRun`, find where `pool, err := accounts.NewPool(...)` is called. Right after, add:

```go
accounts.AssignTiers(pool, cfg.TierWeights, rng.Rand())
```

(Both runners already have a `rng` instance.)

- [ ] **Step 3: Wire `ApplyAll` into SetupState**

Edit `sidecar/internal/fuzz/accounts/setup.go`. After the existing Phase 2 wait at the end of `SetupState`, add:

```go
	// Phase 3: tier-specific account configuration (multisig, regular-key,
	// at-reserve, blackholed). No-op for rich-only pools.
	if err := ApplyAll(client, pool); err != nil {
		return fmt.Errorf("apply tiers: %w", err)
	}
	if err := waitForValidation(client, 2, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for tier setup validation: %w", err)
	}
	return nil
}
```

(Replace the existing `return nil` at the bottom of SetupState. Confirm the diff is clean.)

- [ ] **Step 4: Replace stub RotateTiers with a real refill**

Edit `sidecar/internal/fuzz/accounts/pool.go`. Replace the existing stub:

```go
func RotateTiers(submit *rpcclient.Client, pool *Pool, rng *mathrand.Rand) error {
	_ = rng
	for _, w := range pool.All() {
		_, err := submit.SubmitTxJSON(w.Seed, map[string]any{
			"TransactionType": "AccountSet",
			"Account":         w.ClassicAddress,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
```

with:

```go
// RotateTiers performs tier-specific maintenance: at-reserve wallets that
// have drifted above reserve get re-trimmed; rich wallets that have drifted
// below reserve get topped up from genesis. Other tiers are no-ops (their
// setup is one-shot during SetupState). Called periodically from soak's
// rotation hook.
func RotateTiers(submit *rpcclient.Client, pool *Pool, rng *mathrand.Rand) error {
	_ = rng
	for _, w := range pool.All() {
		switch w.Tier {
		case AtReserve:
			// Re-drain back to reserve_base if balance has grown.
			if err := setupAtReserve(submit, w); err != nil {
				return fmt.Errorf("rotate at-reserve %s: %w", w.ClassicAddress, err)
			}
		case Rich:
			// Top up rich wallets that have drifted near reserve.
			info, err := submit.AccountInfo(w.ClassicAddress)
			if err != nil {
				continue
			}
			balance, err := parseDrops(info.Balance)
			if err != nil {
				continue
			}
			if balance < reserveBaseDrops*2 {
				_, _ = submit.SubmitTxJSON(GenesisSeed, map[string]any{
					"TransactionType": "Payment",
					"Account":         GenesisAddress,
					"Destination":     w.ClassicAddress,
					"Amount":          "10000000000",
				})
			}
		}
	}
	return nil
}
```

The new helpers (`setupAtReserve`, `parseDrops`, `reserveBaseDrops`) all live in `tiers.go`; they're package-private so this works inside `pool.go`.

`GenesisSeed` is exported by `funding.go` (`s████`); confirm by `grep -n "GenesisSeed\|GenesisAddress" sidecar/internal/fuzz/accounts/funding.go`. If only `GenesisAddress` is exported, also export the seed:

```go
// GenesisSeed is the well-known XRPL genesis secret seed.
const GenesisSeed = "snoPBrXtMeMyMHUVTgbuqAfg1SUTb"
```

- [ ] **Step 5: Run all tests**

```bash
cd sidecar
go test ./...
go vet ./...
cd ..
```

Expected: clean. The existing `TestRotateTiers_RecyclesXRP` test was a t.Skip stub; it remains a skip (the new RotateTiers is exercised by ApplyAll's tests + smoke).

- [ ] **Step 6: Commit**

```bash
git add sidecar/internal/fuzz/accounts/setup.go sidecar/internal/fuzz/accounts/pool.go sidecar/internal/fuzz/accounts/funding.go sidecar/internal/fuzz/runners/realtime.go sidecar/internal/fuzz/runners/soak.go
git commit -m "accounts: wire ApplyAll into SetupState; RotateTiers does real maintenance"
```

### Task H7: Smoke test — run soak with mixed tiers

**Files:**
- No new code. Pure verification.

- [ ] **Step 1: Set non-zero tier weights via env (or default in cmd/fuzz)**

For the smoke we'll temporarily edit `cmd/fuzz/main.go::loadConfig` to inject test weights. Simpler: add tier weights as env vars (TIER_RICH, TIER_AT_RESERVE, etc.).

```go
cfg.TierWeights = accounts.TierWeights{
	Rich:       envInt("TIER_RICH", 6),
	AtReserve:  envInt("TIER_AT_RESERVE", 2),
	Multisig:   envInt("TIER_MULTISIG", 1),
	RegularKey: envInt("TIER_REGULAR_KEY", 0), // disabled by default — the multi-step setup is fragile
	Blackholed: envInt("TIER_BLACKHOLED", 1),
}
```

(RegularKey defaults to 0 because if its setup partially succeeds you can lock the account; smoke against it after the basic flow is stable.)

- [ ] **Step 2: Build + run the soak smoke**

```bash
bash scripts/build-sidecar.sh >/dev/null 2>&1
kurtosis enclave rm -f tier-smoke 2>/dev/null
make soak ENCLAVE=tier-smoke ACCOUNTS=10 TX_RATE=2 ROTATE_EVERY=20 2>&1 | tail -3
sleep 90
kurtosis service logs tier-smoke fuzz-soak 2>&1 | grep -E "soak: rotat|setup state:|Tier" | head -10
```

Expected: log lines show `soak: rotating account tiers at N successes`. If the log includes `setup state: apply tiers: ...` errors, that's a real bug — the multi-tier setup needs to succeed under live network conditions. Inspect the failing tier and the rippled response.

- [ ] **Step 3: Tear down**

```bash
kurtosis enclave rm -f tier-smoke
```

- [ ] **Step 4: Push the Phase H batch**

```bash
git push origin main
```

---

## Phase I — xrpl-go-signed `tx_blob` submission

The current submission path uses rippled's `sign_and_submit` (the `submit` method with a `secret` field). This sends the secret over the wire to rippled, which signs and applies. Two reasons to move local:
1. Mutation modes (M2c+) need to corrupt the wire bytes after signing — impossible if rippled signs.
2. Privacy: the secret stops leaving the sidecar.

xrpl-go's `wallet.Sign(tx) (blob, hash, error)` does the signing locally given a fully-formed tx (with Sequence, Fee, LastLedgerSequence, SigningPubKey filled in). Submission becomes `submit{tx_blob}` instead of `submit{tx_json, secret}`.

### Task I1: Sign helper + tx_blob submit

**Files:**
- Create: `sidecar/internal/rpcclient/sign.go`
- Create: `sidecar/internal/rpcclient/sign_test.go`
- Modify: `sidecar/internal/rpcclient/client.go` — add `SubmitTxBlob` method.

- [ ] **Step 1: Failing test**

`sidecar/internal/rpcclient/sign_test.go`:

```go
package rpcclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitTxBlob_PostsBlobMethod(t *testing.T) {
	captured := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string                 `json:"method"`
			Params []map[string]any       `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		captured["method"] = req.Method
		if len(req.Params) > 0 {
			for k, v := range req.Params[0] {
				captured[k] = v
			}
		}
		_, _ = w.Write([]byte(`{"result":{"engine_result":"tesSUCCESS","tx_json":{"hash":"ABC"}}}`))
	}))
	defer srv.Close()

	cl := New(srv.URL)
	res, err := cl.SubmitTxBlob("DEADBEEF")
	if err != nil {
		t.Fatal(err)
	}
	if captured["method"] != "submit" {
		t.Errorf("method = %v, want submit", captured["method"])
	}
	if captured["tx_blob"] != "DEADBEEF" {
		t.Errorf("tx_blob = %v, want DEADBEEF", captured["tx_blob"])
	}
	if res.EngineResult != "tesSUCCESS" {
		t.Errorf("engine = %v", res.EngineResult)
	}
	_ = strings.Builder{}
}
```

- [ ] **Step 2: Run — expect build failure (`SubmitTxBlob` undefined)**

```bash
cd sidecar
go test ./internal/rpcclient/...
```

- [ ] **Step 3: Implement `SubmitTxBlob`**

Edit `sidecar/internal/rpcclient/client.go`. Add (near `SubmitTxJSON`):

```go
// SubmitTxBlob submits a pre-signed transaction blob via the standard
// `submit` RPC. The blob is the hex-encoded XRPL binary form returned by
// xrpl-go's wallet.Sign.
func (c *Client) SubmitTxBlob(blob string) (*SubmitResult, error) {
	raw, err := c.Call("submit", map[string]any{
		"tx_blob": blob,
	})
	if err != nil {
		return nil, err
	}
	return parseSubmitResult(raw)
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/rpcclient/...
```

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/rpcclient/sign_test.go sidecar/internal/rpcclient/client.go
git commit -m "rpcclient: SubmitTxBlob (submit method with tx_blob)"
```

### Task I2: Local-sign helper with autofill

**Files:**
- Create: `sidecar/internal/rpcclient/sign.go`
- Modify: `sidecar/internal/rpcclient/sign_test.go` — add autofill tests.

The xrpl-go `Wallet.Sign(tx)` requires Sequence/Fee/LastLedgerSequence/SigningPubKey already present in the tx map. We provide those by fetching account_info + ledger_current and computing fee from server_state. Wrap as `(*Client).SignLocal(secret, tx)`.

- [ ] **Step 1: Failing test**

Append to `sign_test.go`:

```go
func TestSignLocal_AutofillsSequenceAndFee(t *testing.T) {
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		body, _ := json.Marshal(map[string]any{}) // throwaway init
		_ = body
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Method)
		switch req.Method {
		case "account_info":
			_, _ = w.Write([]byte(`{"result":{"account_data":{"Account":"rAddr","Balance":"1000000000","Sequence":42}}}`))
		case "ledger_current":
			_, _ = w.Write([]byte(`{"result":{"ledger_current_index":100}}`))
		default:
			_, _ = w.Write([]byte(`{"result":{}}`))
		}
	}))
	defer srv.Close()

	cl := New(srv.URL)
	tx := map[string]any{
		"TransactionType": "AccountSet",
		"Account":         "rAddr",
	}
	// snoPBrXtMeMyMHUVTgbuqAfg1SUTb is the genesis secret; use it for the test.
	blob, err := cl.SignLocal("snoPBrXtMeMyMHUVTgbuqAfg1SUTb", tx)
	if err != nil {
		t.Fatal(err)
	}
	if blob == "" {
		t.Fatal("blob is empty")
	}
	wantCalls := map[string]bool{"account_info": false, "ledger_current": false}
	for _, c := range calls {
		if _, ok := wantCalls[c]; ok {
			wantCalls[c] = true
		}
	}
	if !wantCalls["account_info"] || !wantCalls["ledger_current"] {
		t.Errorf("calls = %v, want both account_info and ledger_current", calls)
	}
}
```

- [ ] **Step 2: Run — expect build failure**

- [ ] **Step 3: Implement `SignLocal`**

Create `sidecar/internal/rpcclient/sign.go`:

```go
// Package rpcclient — local signing helper.
//
// SignLocal autofills Sequence / Fee / LastLedgerSequence / SigningPubKey on
// a tx map then hands it to xrpl-go's wallet.Sign to produce a hex-encoded
// tx_blob. The blob can then be submitted via SubmitTxBlob.

package rpcclient

import (
	"encoding/json"
	"fmt"

	"github.com/Peersyst/xrpl-go/xrpl/wallet"
)

// SignLocal autofills the standard tx fields rippled would otherwise fill
// during sign_and_submit, then signs the tx with the given XRPL secret.
// Returns the hex-encoded tx_blob ready for SubmitTxBlob.
func (c *Client) SignLocal(secret string, tx map[string]any) (string, error) {
	w, err := wallet.FromSeed(secret, "")
	if err != nil {
		return "", fmt.Errorf("wallet from seed: %w", err)
	}

	// Account: use the wallet's address if not already set (most callers set it).
	if _, ok := tx["Account"]; !ok {
		tx["Account"] = w.ClassicAddress.String()
	}

	if _, ok := tx["Sequence"]; !ok {
		info, err := c.AccountInfo(tx["Account"].(string))
		if err != nil {
			return "", fmt.Errorf("account_info: %w", err)
		}
		tx["Sequence"] = info.Sequence
	}

	if _, ok := tx["Fee"]; !ok {
		// 10 drops is the default base fee on the test network.
		tx["Fee"] = "10"
	}

	if _, ok := tx["LastLedgerSequence"]; !ok {
		raw, err := c.Call("ledger_current", map[string]any{})
		if err != nil {
			return "", fmt.Errorf("ledger_current: %w", err)
		}
		var lc struct {
			LedgerCurrentIndex int `json:"ledger_current_index"`
		}
		if err := json.Unmarshal(raw, &lc); err != nil {
			return "", fmt.Errorf("parse ledger_current: %w", err)
		}
		tx["LastLedgerSequence"] = lc.LedgerCurrentIndex + 20
	}

	blob, _, err := w.Sign(tx)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return blob, nil
}
```

NOTE on the parse for ledger_current: `Client.Call` returns the `result` field from the rippled response. Confirm by reading `client.go::Call` — it should `json.Unmarshal` the outer wrapper and return `result` raw. If it returns the WHOLE response, adjust the unmarshal target accordingly.

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/rpcclient/...
```

If the test fails because xrpl-go's `wallet.Sign` rejects the test secret, check that "snoPBrXtMeMyMHUVTgbuqAfg1SUTb" is the expected genesis seed format. If the seed needs to be a different format (e.g. base58 hash), use `wallet.New(crypto.SECP256K1())` to generate a fresh wallet for the test instead.

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/rpcclient/sign.go sidecar/internal/rpcclient/sign_test.go
git commit -m "rpcclient: SignLocal autofills sequence/fee/last-ledger then xrpl-go signs"
```

### Task I3: Runner integration with LocalSign flag

**Files:**
- Modify: `sidecar/internal/fuzz/runners/realtime.go`
- Modify: `sidecar/internal/fuzz/runners/soak.go`
- Modify: `sidecar/cmd/fuzz/main.go`

- [ ] **Step 1: Add `LocalSign bool` to `Config`**

Edit `realtime.go`. Add to `Config`:

```go
// LocalSign, when true, signs each tx locally via xrpl-go and submits as
// tx_blob. Default false preserves rippled-side sign_and_submit behavior.
// Required for the byte-mutation generation modes (M2c+).
LocalSign bool
```

- [ ] **Step 2: Branch the submit path**

Find the existing `submit.SubmitTxJSON(tx.Secret, tx.Fields)` call in `Run` and `SoakRun`. Wrap it:

```go
var (
	res *rpcclient.SubmitResult
	err error
)
if cfg.LocalSign {
	blob, signErr := submit.SignLocal(tx.Secret, tx.Fields)
	if signErr != nil {
		atomic.AddInt64(&stats.TxsFailed, 1)
		log.Printf("realtime: sign %s: %v", tx.TransactionType(), signErr)
		continue
	}
	res, err = submit.SubmitTxBlob(blob)
} else {
	res, err = submit.SubmitTxJSON(tx.Secret, tx.Fields)
}
```

Apply the same wrap to soak.go.

- [ ] **Step 3: Read `LOCAL_SIGN` env var in cmd/fuzz**

Edit `loadConfig` in `cmd/fuzz/main.go`. Add:

```go
if os.Getenv("LOCAL_SIGN") == "1" {
	cfg.LocalSign = true
}
```

- [ ] **Step 4: Build + test**

```bash
cd sidecar
go build ./...
go test ./...
go vet ./...
cd ..
```

Expected: clean. The existing TestRun_* and TestSoakRun_* tests use httptest mocks that don't exercise SubmitTxBlob — they continue to pass with `LocalSign: false` (default).

- [ ] **Step 5: Commit**

```bash
git add sidecar/internal/fuzz/runners/realtime.go sidecar/internal/fuzz/runners/soak.go sidecar/cmd/fuzz/main.go
git commit -m "runners: optional LocalSign path (xrpl-go sign + tx_blob submit)"
```

### Task I4: Smoke test — run with LOCAL_SIGN=1

**Files:**
- No new code. Pure verification.

- [ ] **Step 1: Build sidecar**

```bash
bash scripts/build-sidecar.sh >/dev/null 2>&1
```

- [ ] **Step 2: Run soak with LocalSign**

We need to inject LOCAL_SIGN into the env. The cleanest way is via the Starlark launcher; for the smoke we'll temporarily edit `src/sidecar/fuzz.star::launch_soak` to add `LOCAL_SIGN: "1"` to env_vars, run, then revert.

```bash
# Edit src/sidecar/fuzz.star to set LOCAL_SIGN: "1" inside launch_soak's env_vars dict.
# Re-build (no Go changes; just need the new image to mount the updated Starlark).
kurtosis enclave rm -f localsign-smoke 2>/dev/null
make soak ENCLAVE=localsign-smoke ACCOUNTS=5 TX_RATE=2 ROTATE_EVERY=10 2>&1 | tail -3
sleep 90
kurtosis service logs localsign-smoke fuzz-soak 2>&1 | grep -E "soak: rotat|sign |submit" | head -10
```

Expected: at least one `soak: rotating account tiers at N successes` line — proves the LocalSign path successfully submits txs end-to-end. If the log shows `realtime: sign ...: <err>` failures repeatedly, the autofill in `SignLocal` is producing bad values; debug by adding fee/sequence printing.

- [ ] **Step 3: Revert the temporary fuzz.star edit + tear down**

```bash
git checkout src/sidecar/fuzz.star
kurtosis enclave rm -f localsign-smoke
```

- [ ] **Step 4: Push the Phase I batch**

```bash
git push origin main
```

---

## Phase J — goXRPL chaos targets

The chaos `LatencyEvent` and `PartitionEvent` invoke `tc` and `iptables` via `docker exec`. The goXRPL distroless image has neither tool; rippled (Debian-based) does. To exercise goXRPL's network-fault recovery directly, we ship a `goxrpl-tools:latest` image that combines the goXRPL binary with iproute2 + iptables on a Debian slim base. The chaos suite uses this image instead of vanilla `goxrpl:latest`.

### Task J1: Build goxrpl-tools image

**Files:**
- Create: `goxrpl-tools.Dockerfile`
- Create: `scripts/build-goxrpl-tools.sh`

- [ ] **Step 1: Create the Dockerfile**

`goxrpl-tools.Dockerfile`:

```dockerfile
# goxrpl-tools — goXRPL binary + chaos-event tools (iproute2, iptables).
# Built by scripts/build-goxrpl-tools.sh; consumed by the chaos suite.

FROM goxrpl:latest AS bin

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends iproute2 iptables ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=bin /usr/local/bin/goxrpl /usr/local/bin/goxrpl

EXPOSE 5005 5555 6005 6006 51235

ENTRYPOINT ["goxrpl"]
CMD ["server", "--conf", "/etc/goxrpl/xrpld.toml"]
```

- [ ] **Step 2: Create the build script**

`scripts/build-goxrpl-tools.sh`:

```bash
#!/usr/bin/env bash
# Build goxrpl-tools:latest by combining goxrpl:latest with iproute2/iptables.
# Used by the chaos suite to enable LatencyEvent / PartitionEvent against
# goXRPL containers (the vanilla distroless goxrpl image lacks these tools).
set -euo pipefail

cd "$(dirname "$0")/.."

if ! docker image inspect goxrpl:latest --format '{{.Id}}' >/dev/null 2>&1; then
	echo "goxrpl:latest not present — build it from the goXRPL repo first" >&2
	exit 1
fi

docker build -t goxrpl-tools:latest -f goxrpl-tools.Dockerfile .
echo "built goxrpl-tools:latest"
```

```bash
chmod +x scripts/build-goxrpl-tools.sh
```

- [ ] **Step 3: Verify the build**

```bash
bash scripts/build-goxrpl-tools.sh
docker image inspect goxrpl-tools:latest --format '{{.Id}}'
docker run --rm --entrypoint=which goxrpl-tools:latest tc
docker run --rm --entrypoint=which goxrpl-tools:latest iptables
```

Expected: image builds; `which tc` and `which iptables` both print paths.

- [ ] **Step 4: Commit**

```bash
git add goxrpl-tools.Dockerfile scripts/build-goxrpl-tools.sh
git commit -m "scripts: goxrpl-tools image (goxrpl + iproute2 + iptables for chaos)"
```

### Task J2: Pass enable_chaos_tools through Starlark

**Files:**
- Modify: `src/goxrpl/goxrpl.star` — accept `enable_chaos_tools` arg, pick image accordingly.
- Modify: `main.star` — thread the flag through.
- Modify: `src/tests/chaos.star` — pass `enable_chaos_tools=True` (or rely on main.star's default-on for chaos suite).

- [ ] **Step 1: Inspect the existing goxrpl launcher signature**

```bash
sed -n '1,40p' src/goxrpl/goxrpl.star
```

- [ ] **Step 2: Add `enable_chaos_tools` arg to goxrpl.launch**

Edit `src/goxrpl/goxrpl.star`. Find the `launch(...)` function signature and add `enable_chaos_tools=False` as a keyword arg. Inside the body, just before constructing the `ServiceConfig`, add:

```python
goxrpl_image_actual = goxrpl_image
if enable_chaos_tools:
    goxrpl_image_actual = "goxrpl-tools:latest"
```

…and use `goxrpl_image_actual` in the `ServiceConfig(image=...)`.

- [ ] **Step 3: Thread the flag through main.star**

Edit `main.star`. Find the call site `goxrpl_nodes = goxrpl.launch(plan, goxrpl_count, goxrpl_image, network_config)`. Add a new arg derived from the test_suite:

```python
enable_chaos_tools = (test_suite == "chaos")
goxrpl_nodes = goxrpl.launch(plan, goxrpl_count, goxrpl_image, network_config, enable_chaos_tools = enable_chaos_tools)
```

- [ ] **Step 4: Starlark syntax check**

```bash
kurtosis enclave rm -f j2-syntax 2>/dev/null
kurtosis run --enclave j2-syntax . '{"test_suite":"propagation","goxrpl_count":1,"rippled_count":2}' 2>&1 | tail -10
kurtosis enclave rm -f j2-syntax
```

Expected: parses + brings up enclave (default path: `enable_chaos_tools=False`, uses vanilla goxrpl:latest).

- [ ] **Step 5: Commit**

```bash
git add src/goxrpl/goxrpl.star main.star
git commit -m "confluence: enable_chaos_tools flag selects goxrpl-tools image for chaos suite"
```

### Task J3: Live smoke — chaos against goxrpl

**Files:**
- No new code. Pure verification.

The earlier F3.11 smoke confirmed chaos events fire correctly when targeting a rippled container. This task confirms the same path works for a goXRPL container under the goxrpl-tools image.

- [ ] **Step 1: Build everything**

```bash
bash scripts/build-sidecar.sh >/dev/null 2>&1
bash scripts/build-goxrpl-tools.sh
```

- [ ] **Step 2: Run chaos suite targeting goxrpl-0**

```bash
echo '[{"step":30,"recover_after":20,"type":"latency","container":"goxrpl-0","iface":"eth0","delay_ms":150}]' > /tmp/chaos-goxrpl-test.json

kurtosis enclave rm -f chaos-goxrpl-smoke 2>/dev/null
SCHEDULE=$(cat /tmp/chaos-goxrpl-test.json | tr -d '\n' | sed 's/"/\\"/g')
kurtosis run --enclave chaos-goxrpl-smoke . \
  "{\"test_suite\":\"chaos\",\"goxrpl_count\":2,\"rippled_count\":3,\"chaos_args\":{\"schedule\":\"$SCHEDULE\",\"tx_rate\":3,\"accounts\":3,\"rotate_every\":10}}" 2>&1 | tail -3
```

Wait for `chaos: apply latency:goxrpl-0` to fire (~3 minutes). Then check the actual qdisc was added:

```bash
sleep 180
docker exec "$(docker ps --filter name=goxrpl-0 --format '{{.ID}}' | head -1)" tc qdisc show dev eth0 2>&1
```

Expected: `qdisc netem 8001: root refcnt 2 limit 1000 delay 150ms` (or similar).

After RecoverAfter (20 ticks ≈ 60s later) the qdisc should be removed:

```bash
sleep 90
docker exec "$(docker ps --filter name=goxrpl-0 --format '{{.ID}}' | head -1)" tc qdisc show dev eth0 2>&1
```

Expected: no `netem` qdisc — the recover removed it.

- [ ] **Step 3: Tear down**

```bash
kurtosis enclave rm -f chaos-goxrpl-smoke
```

- [ ] **Step 4: Push the Phase J batch**

```bash
git push origin main
```

### Task J4: Final review

**Files:**
- No code. Pure verification.

- [ ] **Step 1: Full test sweep**

```bash
cd sidecar
go test ./...
go vet ./...
cd ..
```

- [ ] **Step 2: Spot-check the diff**

```bash
git log --oneline 9445e6b..HEAD  # original chaos baseline through end-of-followups
git diff --stat 9445e6b..HEAD | tail -25
```

Confirm:
- Phase G touched dashboard/static/app.js (sort change), Makefile (loop targets), scripts/corpus-pull-loop.sh (new).
- Phase H touched accounts/{tiers.go,tiers_test.go,pool.go,setup.go,keys.go,funding.go} + runners/{realtime.go,soak.go}.
- Phase I touched rpcclient/{sign.go,sign_test.go,client.go} + runners/{realtime.go,soak.go} + cmd/fuzz/main.go.
- Phase J touched goxrpl-tools.Dockerfile (new), scripts/build-goxrpl-tools.sh (new), src/goxrpl/goxrpl.star, main.star.

- [ ] **Step 3: Final push if anything is unpushed**

```bash
git push origin main
```

---

## Self-review checklist

1. **Spec coverage**:
   - Item 5 (chaos layer in dashboard) → Task G1 ✓
   - Item 6 (periodic corpus extraction) → Task G2 ✓
   - Item 7 (multi-tier accounts) → Tasks H1–H7 ✓
   - Item 9 (xrpl-go-signed tx_blob) → Tasks I1–I4 ✓
   - Item 10 (goXRPL chaos targets) → Tasks J1–J4 ✓

2. **Placeholder scan**:
   - I3 Step 2 says "find the existing `submit.SubmitTxJSON(tx.Secret, tx.Fields)` call ... wrap it" — no `<TBD>` placeholder, the code is shown.
   - H1 Step 3 inserts a single field; the rest of the struct stays as-is. No "fill in details."
   - H6 Step 4's RotateTiers replacement code shows the full new function body.
   - All env-var defaults (TIER_RICH=6, TIER_AT_RESERVE=2, etc.) are explicit.
   - The note in I2 about "If the test fails because xrpl-go's wallet.Sign rejects the test secret" is a contingency for SDK-version drift, not a placeholder — it includes the workaround.

3. **Type/name consistency**:
   - `Tier` enum values (Rich, AtReserve, Multisig, RegularKey, Blackholed) consistent across H1–H6 and the runner config.
   - `TierWeights` field names match between H1 (definition) and H6 (consumption in cmd/fuzz).
   - `submitter` interface (used in tier setup tests + ApplyAll) consistent — minimal: SubmitTxJSON + AccountInfo. The full *rpcclient.Client satisfies it; tests inject stubSubmit.
   - `SignLocal(secret, tx)` and `SubmitTxBlob(blob)` signatures consistent in I1, I2, I3.
   - `LocalSign bool` field in Config used identically in realtime.go and soak.go.
   - `enable_chaos_tools` Starlark arg name consistent in goxrpl.star and main.star.
   - `goxrpl-tools:latest` image tag consistent in J1, J2 — and matches the Dockerfile output.

4. **Out-of-scope** (deliberately deferred):
   - Tier-aware tx generators (e.g. always-fail tx from blackholed accounts). Adding the tier classification + setup is the foundation; specialized generators are an M5+ follow-up.
   - Multi-key signing for Multisig accounts. The current SubmitTxJSON path single-signs with the master key — multisigned tx require a separate generator that builds the SigningPubKey-less tx + assembles SignerEntries. M5+ work.
   - Real LastLedgerSequence buffer tuning. We hard-code +20; under load you may want this configurable.
   - Random byte-corruption mutation via the new LocalSign path. Phase I unblocks it but doesn't add the mutation logic — that's the existing M2c mutator working through a different submission path.
   - Docker socket forwarding for chaos events on macOS. Same as F3 — operator-level concern.
