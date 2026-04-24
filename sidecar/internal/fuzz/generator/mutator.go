package generator

import (
	"encoding/binary"
	"encoding/hex"
	mathrand "math/rand/v2"
)

// Mutator applies a single-field mutation to a generator.Tx with some
// probability, producing an invalid-but-plausible tx that exercises
// validation code paths. Each mutation strategy is deterministic from the
// RNG — a given seed always produces the same mutation sequence.
//
// Strategies are registered once at construction. Not every strategy applies
// to every tx; the Mutator tries strategies in order, falling back to "no-op"
// when none applies (in which case Maybe returns (tx, false)).
type Mutator struct {
	strategies []mutStrategy
}

// mutStrategy is the interface every mutation strategy satisfies.
type mutStrategy interface {
	// name returns a short identifier for logs/metrics.
	name() string
	// canApply reports whether this strategy has an eligible field in tx.
	canApply(tx *Tx) bool
	// mutate returns a deep-enough copy of tx with one field mutated.
	// Callers must have confirmed canApply first.
	mutate(r *mathrand.Rand, tx *Tx) *Tx
}

// NewMutator returns a Mutator with the default strategy set. Callers who
// want a custom set (e.g., a test that exercises just one strategy) can
// construct one directly via apply() — that stays unexported.
func NewMutator() *Mutator {
	return &Mutator{strategies: []mutStrategy{
		mutDeleteField{},
		mutCorruptDrops{},
		mutCorruptAddress{},
		mutOverflowUint32{},
		mutZeroUint32{},
	}}
}

// Maybe returns a mutated tx with probability rate (clamped to [0,1]). The
// second return value reports whether a mutation was actually applied.
func (m *Mutator) Maybe(r *mathrand.Rand, tx *Tx, rate float64) (*Tx, bool) {
	if rate <= 0 {
		return tx, false
	}
	if rate < 1.0 && r.Float64() >= rate {
		return tx, false
	}

	// Find eligible strategies and pick one.
	eligible := make([]mutStrategy, 0, len(m.strategies))
	for _, s := range m.strategies {
		if s.canApply(tx) {
			eligible = append(eligible, s)
		}
	}
	if len(eligible) == 0 {
		return tx, false
	}
	choice := eligible[r.IntN(len(eligible))]
	return choice.mutate(r, tx), true
}

// apply forces a specific strategy. For tests only.
func (m *Mutator) apply(r *mathrand.Rand, tx *Tx, s mutStrategy) (*Tx, bool) {
	if !s.canApply(tx) {
		return tx, false
	}
	return s.mutate(r, tx), true
}

// cloneTx deep-copies the Fields map so the original is unchanged.
func cloneTx(tx *Tx) *Tx {
	fields := make(map[string]any, len(tx.Fields))
	for k, v := range tx.Fields {
		fields[k] = v
	}
	return &Tx{Fields: fields, Secret: tx.Secret}
}

// -------- strategies --------

// mutDeleteField removes one non-required field at random.
type mutDeleteField struct{}

func (mutDeleteField) name() string { return "delete_field" }
func (mutDeleteField) canApply(tx *Tx) bool {
	// Need at least one deletable (non-TransactionType) field.
	for k := range tx.Fields {
		if k != "TransactionType" {
			return true
		}
	}
	return false
}
func (mutDeleteField) mutate(r *mathrand.Rand, tx *Tx) *Tx {
	out := cloneTx(tx)
	deletable := make([]string, 0, len(out.Fields))
	for k := range out.Fields {
		if k != "TransactionType" {
			deletable = append(deletable, k)
		}
	}
	// Deterministic pick: sort and index.
	sortStrings(deletable)
	victim := deletable[r.IntN(len(deletable))]
	delete(out.Fields, victim)
	return out
}

// mutCorruptDrops replaces an XRP drops string field with a suspicious value.
type mutCorruptDrops struct{}

var dropsFieldNames = []string{"Amount", "TakerPays", "TakerGets", "Fee", "SendMax", "DeliverMin"}

func (mutCorruptDrops) name() string { return "corrupt_drops" }
func (mutCorruptDrops) canApply(tx *Tx) bool {
	for _, f := range dropsFieldNames {
		if v, ok := tx.Fields[f]; ok {
			if _, isString := v.(string); isString {
				return true
			}
		}
	}
	return false
}
func (mutCorruptDrops) mutate(r *mathrand.Rand, tx *Tx) *Tx {
	out := cloneTx(tx)
	candidates := make([]string, 0)
	for _, f := range dropsFieldNames {
		if v, ok := out.Fields[f]; ok {
			if _, isString := v.(string); isString {
				candidates = append(candidates, f)
			}
		}
	}
	sortStrings(candidates)
	victim := candidates[r.IntN(len(candidates))]

	corrupt := []string{"-1", "0", "9999999999999999999999", "not-a-number", ""}
	out.Fields[victim] = corrupt[r.IntN(len(corrupt))]
	return out
}

// mutCorruptAddress corrupts an account-typed field.
type mutCorruptAddress struct{}

var addrFieldNames = []string{"Account", "Destination", "RegularKey", "Issuer"}

func (mutCorruptAddress) name() string { return "corrupt_address" }
func (mutCorruptAddress) canApply(tx *Tx) bool {
	for _, f := range addrFieldNames {
		if v, ok := tx.Fields[f]; ok {
			if s, isString := v.(string); isString && s != "" {
				return true
			}
		}
	}
	return false
}
func (mutCorruptAddress) mutate(r *mathrand.Rand, tx *Tx) *Tx {
	out := cloneTx(tx)
	candidates := make([]string, 0)
	for _, f := range addrFieldNames {
		if v, ok := out.Fields[f]; ok {
			if s, isString := v.(string); isString && s != "" {
				candidates = append(candidates, f)
			}
		}
	}
	sortStrings(candidates)
	victim := candidates[r.IntN(len(candidates))]

	// 32 random hex chars — valid-looking shape, invalid as an address.
	// math/rand/v2.Rand has no Read method; fill via two Uint64 calls.
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], r.Uint64())
	binary.LittleEndian.PutUint64(buf[8:16], r.Uint64())
	out.Fields[victim] = "r" + hex.EncodeToString(buf[:])[:28]
	return out
}

// mutOverflowUint32 replaces a uint32 field with math.MaxUint32.
type mutOverflowUint32 struct{}

var uint32FieldNames = []string{"SetFlag", "ClearFlag", "TicketCount", "Flags", "DestinationTag", "SourceTag", "Sequence"}

func (mutOverflowUint32) name() string { return "overflow_uint32" }
func (mutOverflowUint32) canApply(tx *Tx) bool {
	for _, f := range uint32FieldNames {
		if v, ok := tx.Fields[f]; ok {
			if _, isU32 := v.(uint32); isU32 {
				return true
			}
		}
	}
	return false
}
func (mutOverflowUint32) mutate(r *mathrand.Rand, tx *Tx) *Tx {
	out := cloneTx(tx)
	candidates := make([]string, 0)
	for _, f := range uint32FieldNames {
		if v, ok := out.Fields[f]; ok {
			if _, isU32 := v.(uint32); isU32 {
				candidates = append(candidates, f)
			}
		}
	}
	sortStrings(candidates)
	victim := candidates[r.IntN(len(candidates))]
	out.Fields[victim] = uint32(0xFFFFFFFF)
	return out
}

// mutZeroUint32 zeros out a uint32 field. Useful for TicketCount (spec says
// 1..250) and similar non-zero-required counts.
type mutZeroUint32 struct{}

func (mutZeroUint32) name() string { return "zero_uint32" }
func (mutZeroUint32) canApply(tx *Tx) bool {
	return mutOverflowUint32{}.canApply(tx)
}
func (mutZeroUint32) mutate(r *mathrand.Rand, tx *Tx) *Tx {
	out := cloneTx(tx)
	candidates := make([]string, 0)
	for _, f := range uint32FieldNames {
		if v, ok := out.Fields[f]; ok {
			if _, isU32 := v.(uint32); isU32 {
				candidates = append(candidates, f)
			}
		}
	}
	sortStrings(candidates)
	victim := candidates[r.IntN(len(candidates))]
	out.Fields[victim] = uint32(0)
	return out
}

// sortStrings sorts in place (avoids pulling in sort package since we need
// only string slices here).
func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
