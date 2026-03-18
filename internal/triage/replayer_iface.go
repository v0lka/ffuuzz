package triage

import (
	"context"

	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

// SessionReplayer abstracts session replay for testability.
// *replayer.Replayer satisfies this interface.
type SessionReplayer interface {
	ReplaySession(ctx context.Context, session model.RecordingSession, baseURL string, wctx *replayer.WorkerContext, extractionRules []replayer.ExtractionRule) ([]replayer.ExchangeResult, error)
}
