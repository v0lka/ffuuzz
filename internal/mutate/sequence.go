package mutate

import (
	"math/rand"

	"ffuuzz/internal/model"
)

// SeqMutator implements SequenceMutator for exchange sequence mutations.
type SeqMutator struct{}

func (m *SeqMutator) Mutate(exs []model.Exchange, rng *rand.Rand, intensity float64) SequenceMutationResult {
	if len(exs) <= 1 {
		return SequenceMutationResult{Exchanges: exs, Operators: []string{"seq:noop"}}
	}

	op := rng.Intn(4)
	var opName string
	result := copyExchanges(exs)

	switch op {
	case 0:
		opName = "seq:drop"
		result = SeqDrop(result, rng)
	case 1:
		opName = "seq:duplicate"
		result = SeqDuplicate(result, rng)
	case 2:
		opName = "seq:swap"
		result = SeqSwap(result, rng)
	case 3:
		// Per-step: apply a primitive mutation to one random exchange
		idx := rng.Intn(len(result))
		p := &PrimitiveMutator{}
		r := p.Mutate(result[idx], rng, intensity)
		result[idx] = r.Exchange
		opName = "seq:perstep(" + r.Operators[0] + ")"
	}

	return SequenceMutationResult{Exchanges: result, Operators: []string{opName}}
}

// SeqDrop removes one exchange from the sequence (never the first).
func SeqDrop(exs []model.Exchange, rng *rand.Rand) []model.Exchange {
	if len(exs) <= 1 {
		return exs
	}
	// Don't drop the first exchange (index 0) - it may be a login/setup step
	idx := 1 + rng.Intn(len(exs)-1)
	return append(exs[:idx], exs[idx+1:]...)
}

// SeqDuplicate duplicates one exchange in the sequence.
func SeqDuplicate(exs []model.Exchange, rng *rand.Rand) []model.Exchange {
	if len(exs) == 0 {
		return exs
	}
	idx := rng.Intn(len(exs))
	result := make([]model.Exchange, 0, len(exs)+1)
	result = append(result, exs[:idx+1]...)
	result = append(result, exs[idx]) // duplicate
	result = append(result, exs[idx+1:]...)
	return result
}

// SeqSwap swaps two adjacent exchanges.
func SeqSwap(exs []model.Exchange, rng *rand.Rand) []model.Exchange {
	if len(exs) < 2 {
		return exs
	}
	i := rng.Intn(len(exs) - 1)
	exs[i], exs[i+1] = exs[i+1], exs[i]
	return exs
}

func copyExchanges(exs []model.Exchange) []model.Exchange {
	result := make([]model.Exchange, len(exs))
	copy(result, exs)
	return result
}
