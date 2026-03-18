// Package logging provides structured logging configuration using zerolog.
package logging

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

// New creates a zerolog.Logger writing JSON to the given writer.
// If w is nil, os.Stderr is used.
func New(w io.Writer) zerolog.Logger {
	if w == nil {
		w = os.Stderr
	}
	return zerolog.New(w).With().Timestamp().Logger()
}

// WithRequestID returns a child logger with the request_id field set.
func WithRequestID(logger zerolog.Logger, requestID string) zerolog.Logger {
	return logger.With().Str("request_id", requestID).Logger()
}

// WithCampaignID returns a child logger with the campaign_id field set.
func WithCampaignID(logger zerolog.Logger, campaignID string) zerolog.Logger {
	return logger.With().Str("campaign_id", campaignID).Logger()
}

// WithRecordingID returns a child logger with the recording_id field set.
func WithRecordingID(logger zerolog.Logger, recordingID string) zerolog.Logger {
	return logger.With().Str("recording_id", recordingID).Logger()
}
