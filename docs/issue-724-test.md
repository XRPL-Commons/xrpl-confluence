# Issue #724 — consensus rejoin double-fault: test kit

`#724` is a goXRPL-specific **rejoin** defect. Catch-up itself is sound (a node
behind a *validated* tip catches up fine). The bug appears after a **quorum-loss
double-fault**: when both quorum-critical go-xrpl fall behind at once the network
loses quorum and halts (no validated tip); on recovery a go-xrpl can wedge in
`wrongLedger` / `tracking`, unable to adopt the network's validated tip
(validated frozen, `our_pos_seq=0`). rippled recovers from the identical fault;
goXRPL (sometimes) does not.

## Reproducer (self-contained chaos scenario)

`scenarios/issue724-doublefault.yaml` — 3 rippled + 2 go-xrpl (quorum 4-of-5),
`workload.kind: fuzz` + a `chaos.schedule` that **restarts both go-xrpl at the
same step** several times. The `restart` event stops the container and brings it
back `recover_after` steps later (steps = soak-loop / batch-close ticks, so they
keep advancing even while the network is halted).

```bash
# Linux (CI / the VM) — docker.sock is available to the enclave natively:
./bin/confluence up -f scenarios/issue724-doublefault.yaml --enclave xrpl-724 --package .

# macOS only — chaos needs the docker-socket-proxy first, else the runner logs
# "chaos: NetworkRuntime disabled" and faults never fire:
make docker-proxy
./bin/confluence up -f scenarios/issue724-doublefault.yaml --enclave xrpl-724 --package .

# watch:
./bin/confluence status   --enclave xrpl-724
./bin/confluence findings  --enclave xrpl-724
```

### Pass / fail
- **WEDGE reproduced (bug present):** a `state_divergence` finding
  `no validated_seq advance across 5 nodes ...`, and `confluence status` shows a
  go-xrpl stuck in `tracking` at a frozen `validated` seq while the others moved on.
- **Healthy:** after each double-fault both go-xrpl return to `full`/`proposing`
  and the network resumes lockstep; no liveness finding.

The wedge is **stochastic** — re-run a few times (the schedule fires the fault 4×
per run to raise the odds). Increase `tx_rate` or add more schedule entries to
push harder.

## Debugging
On a wedged go-xrpl: `docker logs <goxrpl-N>` and look for `mode=wrongLedger`,
`Cannot acquire network ledger`, repeated `adopted ledger ... seq=<high>` with a
frozen `complete=1-<low>` (disconnected islands), and `ct-avalanche ...
avalanche_state=stuck our_pos_seq=0`. The relevant code is
`internal/consensus/rcl/engine.go` (`OnLedger`, `checkLedger`, `handleWrongLedger`)
— the node must abandon a forked LCL and jump to the validation-preferred tip
(rippled `checkAccept`/`setValidLedger`). Partial fix in go-xrpl PR #773.

## Controls (to classify any wedge)
- `scenarios/soak-isolation-4r1g.yaml` — 4 rippled + 1 go-xrpl. Network validates
  WITHOUT go-xrpl, so a single lagging go-xrpl has a real validated tip to catch
  up to. Pause/unpause the go-xrpl (`docker pause`) → it should recover quickly.
  Isolates catch-up (works) from the rejoin double-fault (the bug).
- `scenarios/soak-5r1g.yaml` — 5 rippled + 1 go-xrpl (quorum 5). Pause 2 rippled →
  halt → unpause → all recover. Confirms rippled handles the double-fault, so any
  go-xrpl non-recovery is goXRPL-specific.
