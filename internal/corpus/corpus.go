// Package corpus loads seed recordings and computes baseline metrics for campaigns.
package corpus

import (
	"context"
	"fmt"
	"sort"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

// RecordingReader provides read access to recording sessions.
type RecordingReader interface {
	GetByID(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error)
}

// CampaignReader provides read access to campaigns.
type CampaignReader interface {
	GetByID(ctx context.Context, id string) (*model.Campaign, error)
}

// Manager provides seed loading and baseline computation for campaigns.
type Manager struct {
	recordings RecordingReader
	campaigns  CampaignReader
	logger     zerolog.Logger
}

func NewManager(recordings RecordingReader, campaigns CampaignReader, logger zerolog.Logger) *Manager {
	return &Manager{recordings: recordings, campaigns: campaigns, logger: logger}
}

// GetSeeds loads all recording sessions linked to a campaign, including their entries.
func (m *Manager) GetSeeds(ctx context.Context, campaignID string) ([]model.RecordingSession, error) {
	campaign, err := m.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("get campaign: %w", err)
	}
	if campaign == nil {
		return nil, fmt.Errorf("campaign %s not found", campaignID)
	}

	var seeds []model.RecordingSession
	for _, rid := range campaign.RecordingIDs {
		sess, err := m.recordings.GetByID(ctx, rid, true, 0)
		if err != nil {
			return nil, fmt.Errorf("get recording %s: %w", rid, err)
		}
		if sess == nil {
			m.logger.Warn().Str("recording_id", rid).Msg("recording not found, skipping")
			continue
		}
		seeds = append(seeds, *sess)
	}
	return seeds, nil
}

// BaselineEntry holds per-endpoint baseline latency data.
type BaselineEntry struct {
	Method     string
	Endpoint   string
	P50Ms      int64
	StatusCode int
}

// ComputeBaseline calculates per-endpoint p50 latency from recording sessions.
func ComputeBaseline(sessions []model.RecordingSession) map[string]BaselineEntry {
	type key struct {
		method   string
		endpoint string
	}

	latencies := make(map[key][]int64)
	statuses := make(map[key]int)

	for _, sess := range sessions {
		for _, ex := range sess.Entries {
			k := key{method: ex.Request.Method, endpoint: sess.Target.Path}
			latencies[k] = append(latencies[k], ex.DurationMs)
			statuses[k] = ex.Response.Status
		}
	}

	result := make(map[string]BaselineEntry)
	for k, lats := range latencies {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		n := len(lats)
		var p50 int64
		if n%2 == 0 {
			p50 = (lats[n/2-1] + lats[n/2]) / 2
		} else {
			p50 = lats[n/2]
		}
		mapKey := k.method + "|" + k.endpoint
		result[mapKey] = BaselineEntry{
			Method:     k.method,
			Endpoint:   k.endpoint,
			P50Ms:      p50,
			StatusCode: statuses[k],
		}
	}
	return result
}
