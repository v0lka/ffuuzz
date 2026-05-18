// Package report builds aggregate summaries from recorded transactions.
package report

import (
	"net/url"

	"ffuuzz/internal/recorder"
)

// Summary aggregates statistics from a set of recorded transactions.
type Summary struct {
	Total    int            `json:"total"`
	ByMethod map[string]int `json:"by_method"`
	ByStatus map[int]int    `json:"by_status"`
	ByHost   map[string]int `json:"by_host"`
}

// BuildSummary aggregates statistics from a set of recorded transactions.
func BuildSummary(records []recorder.TxRecord) Summary {
	s := Summary{
		ByMethod: make(map[string]int),
		ByStatus: make(map[int]int),
		ByHost:   make(map[string]int),
	}

	for _, tx := range records {
		s.Total++
		s.ByMethod[tx.Method]++
		s.ByStatus[tx.RespStatus]++

		u, err := url.Parse(tx.URL)
		if err == nil && u.Host != "" {
			s.ByHost[u.Host]++
		}
	}

	return s
}
