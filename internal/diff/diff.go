// Package diff provides structural comparison of recorded HTTP transactions.
package diff

import "ffuuzz/internal/recorder"

type FieldDiff struct {
	Field string      `json:"field"`
	Old   interface{} `json:"old"`
	New   interface{} `json:"new"`
}

// TxDiff describes the differences between two recorded transactions.
type TxDiff struct {
	RequestIDA string      `json:"request_id_a"`
	RequestIDB string      `json:"request_id_b"`
	Diffs      []FieldDiff `json:"diffs"`
}

func DiffTxRecords(a, b recorder.TxRecord) TxDiff {
	res := TxDiff{
		RequestIDA: a.RequestID,
		RequestIDB: b.RequestID,
	}

	if a.URL != b.URL {
		res.Diffs = append(res.Diffs, FieldDiff{
			Field: "url",
			Old:   a.URL,
			New:   b.URL,
		})
	}

	if a.RespStatus != b.RespStatus {
		res.Diffs = append(res.Diffs, FieldDiff{
			Field: "resp_status",
			Old:   a.RespStatus,
			New:   b.RespStatus,
		})
	}

	return res
}
