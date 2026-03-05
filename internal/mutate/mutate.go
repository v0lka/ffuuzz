package mutate

import "ffuuzz/internal/recorder"

type Mutator interface {
	Mutate(tx recorder.TxRecord) recorder.TxRecord
}

type Func func(tx recorder.TxRecord) recorder.TxRecord

func (f Func) Mutate(tx recorder.TxRecord) recorder.TxRecord {
	return f(tx)
}

var Noop = Func(func(tx recorder.TxRecord) recorder.TxRecord {
	return tx
})
