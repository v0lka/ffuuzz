package endpoint

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: ""},
		{name: "root", path: "/", want: "/"},
		{name: "no params", path: "/api/users", want: "/api/users"},
		{name: "already normalised", path: "/api/users/{_}", want: "/api/users/{_}"},
		{name: "trailing slash", path: "/api/users/", want: "/api/users/"},

		{name: "numeric id", path: "/api/users/123", want: "/api/users/{_}"},
		{name: "numeric zero", path: "/api/users/0", want: "/api/users/{_}"},
		{name: "multiple numeric", path: "/api/users/123/posts/456", want: "/api/users/{_}/posts/{_}"},

		{name: "uuid lowercase", path: "/api/users/550e8400-e29b-41d4-a716-446655440000", want: "/api/users/{_}"},
		{name: "uuid mixed case", path: "/api/users/550E8400-E29B-41D4-A716-446655440000", want: "/api/users/{_}"},

		{name: "hex 8 chars", path: "/api/files/a1b2c3d4", want: "/api/files/{_}"},
		{name: "hex 16 chars", path: "/api/files/a1b2c3d4e5f6a7b8", want: "/api/files/{_}"},
		{name: "hex short 6 chars", path: "/api/ab12cd", want: "/api/ab12cd"},

		{name: "hashed js", path: "/assets/app.a1b2c3d4.js", want: "/assets/{_}"},
		{name: "hashed css", path: "/assets/style.4f8e2a.css", want: "/assets/{_}"},
		{name: "normal file", path: "/assets/logo.png", want: "/assets/logo.png"},
		{name: "dotfile no hash", path: "/assets/.gitignore", want: "/assets/.gitignore"},

		{name: "base64 token", path: "/api/auth/dGhpcyBpcyBhIGxvbmc=", want: "/api/auth/{_}"},                   // 24 chars, mixed classes → token
		{name: "jwt-like token", path: "/api/verify/eyJhbGciOiJIUzI1NiJ9", want: "/api/verify/{_}"},             // 24 chars, mixed classes
		{name: "short token", path: "/api/verify/abc123", want: "/api/verify/abc123"},                           // < 16 chars, not a token
		{name: "long alpha only", path: "/api/verify/abcdefghijklmnop", want: "/api/verify/abcdefghijklmnop"},   // only lowercase, single class
		{name: "long mixed token", path: "/api/verify/AbCdEfGh12345678", want: "/api/verify/{_}"},               // 16 chars, mixed
		{name: "url-safe base64", path: "/api/tokens/dGhpc19pc19hX3Rva2Vu_test-value", want: "/api/tokens/{_}"}, // 35 chars, mixed with _ and -

		{name: "rest deep", path: "/api/v2/users/123/posts/456/comments/789", want: "/api/v2/users/{_}/posts/{_}/comments/{_}"},
		{name: "static unchanged", path: "/static/logo.png", want: "/static/logo.png"},
		{name: "version prefix", path: "/v1/api/users", want: "/v1/api/users"},
		{name: "idempotent double", path: "/api/users/{_}/posts/{_}", want: "/api/users/{_}/posts/{_}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePath(tt.path)
			if got != tt.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizePath_Idempotent(t *testing.T) {
	paths := []string{
		"/api/users/123",
		"/api/users/550e8400-e29b-41d4-a716-446655440000/posts/456",
		"/assets/app.a1b2c3d4.js",
		"/api/verify/eyJhbGciOiJIUzI1NiJ9",
	}
	for _, p := range paths {
		first := NormalizePath(p)
		second := NormalizePath(first)
		if first != second {
			t.Errorf("not idempotent: NormalizePath(%q) = %q, NormalizePath(%q) = %q", p, first, first, second)
		}
	}
}

func TestSplitPathSegments(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/api/users/123", []string{"api", "users", "123"}},
		{"/", nil},
		{"", nil},
		{"/a//b/", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := SplitPathSegments(tt.path)
		if len(got) != len(tt.want) {
			t.Errorf("SplitPathSegments(%q) = %v, want %v", tt.path, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("SplitPathSegments(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
			}
		}
	}
}
