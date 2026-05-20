package api

import (
	"strings"
	"testing"

	"ffuuzz/internal/model"
)

// baseValidConfig returns a CampaignConfig populated with the minimum set of
// fields required to pass validateCampaignConfig. Tests mutate the returned
// value to focus on a single validation rule.
func baseValidConfig() model.CampaignConfig {
	return model.CampaignConfig{
		Target: model.TargetURL{BaseURL: "http://example.com"},
		Limits: model.CampaignLimits{
			Workers:      1,
			RPS:          1,
			MaxTests:     10,
			ReqTimeoutMs: 1000,
		},
		Mutations: model.MutationConfig{Intensity: 0.5},
	}
}

func TestValidateCampaignConfig_NewLimitFields(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*model.CampaignConfig)
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "defaults pass",
			mutate:  func(_ *model.CampaignConfig) {},
			wantErr: false,
		},
		{
			name: "min_tests_per_endpoint zero ok",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.MinTestsPerEndpoint = 0
			},
			wantErr: false,
		},
		{
			name: "min_tests_per_endpoint positive ok",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.MinTestsPerEndpoint = 5
			},
			wantErr: false,
		},
		{
			name: "min_tests_per_endpoint negative rejected",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.MinTestsPerEndpoint = -1
			},
			wantErr:   true,
			errSubstr: "min_tests_per_endpoint",
		},
		{
			name: "sequence_share zero ok",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.SequenceShare = 0
			},
			wantErr: false,
		},
		{
			name: "sequence_share one ok",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.SequenceShare = 1.0
			},
			wantErr: false,
		},
		{
			name: "sequence_share above one rejected",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.SequenceShare = 1.1
			},
			wantErr:   true,
			errSubstr: "sequence_share",
		},
		{
			name: "sequence_share negative rejected",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.SequenceShare = -0.1
			},
			wantErr:   true,
			errSubstr: "sequence_share",
		},
		{
			name: "endpoint weight empty path rejected",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.EndpointWeights = []model.EndpointWeightOverride{
					{Method: "GET", Path: "", Weight: 1.0},
				}
			},
			wantErr:   true,
			errSubstr: "endpoint_weights[0].path",
		},
		{
			name: "endpoint weight negative weight rejected",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.EndpointWeights = []model.EndpointWeightOverride{
					{Method: "GET", Path: "/api/users", Weight: -1.0},
				}
			},
			wantErr:   true,
			errSubstr: "endpoint_weights[0].weight",
		},
		{
			name: "endpoint weight unknown method rejected",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.EndpointWeights = []model.EndpointWeightOverride{
					{Method: "QUERY", Path: "/api/users", Weight: 1.0},
				}
			},
			wantErr:   true,
			errSubstr: "endpoint_weights[0].method",
		},
		{
			name: "endpoint weight valid",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.EndpointWeights = []model.EndpointWeightOverride{
					{Method: "GET", Path: "/api/users", Weight: 1.0},
				}
			},
			wantErr: false,
		},
		{
			name: "endpoint weight wildcard method allowed",
			mutate: func(c *model.CampaignConfig) {
				c.Limits.EndpointWeights = []model.EndpointWeightOverride{
					{Method: "", Path: "/api/users", Weight: 2.0},
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValidConfig()
			tt.mutate(&cfg)
			err := validateCampaignConfig(&cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateCampaignConfig_NormalizesEndpointWeight(t *testing.T) {
	cfg := baseValidConfig()
	cfg.Limits.EndpointWeights = []model.EndpointWeightOverride{
		{Method: "get", Path: "/api/users/123/posts/abc12345", Weight: 1.0},
	}

	if err := validateCampaignConfig(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cfg.Limits.EndpointWeights[0]
	if got.Method != "GET" {
		t.Errorf("method not uppercased: got %q want %q", got.Method, "GET")
	}
	const want = "/api/users/{_}/posts/{_}"
	if got.Path != want {
		t.Errorf("path not normalised: got %q want %q", got.Path, want)
	}
}
