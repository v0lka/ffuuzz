package endpoint

import (
	"testing"

	"ffuuzz/internal/model"
)

func TestNewKey_NormalizesPath(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   Key
	}{
		{
			name:   "raw numeric id",
			method: "GET",
			path:   "/api/users/123",
			want:   Key{Method: "GET", Path: "/api/users/{_}"},
		},
		{
			name:   "uuid segment",
			method: "POST",
			path:   "/api/items/550e8400-e29b-41d4-a716-446655440000",
			want:   Key{Method: "POST", Path: "/api/items/{_}"},
		},
		{
			name:   "already normalised",
			method: "DELETE",
			path:   "/api/users/{_}/posts/{_}",
			want:   Key{Method: "DELETE", Path: "/api/users/{_}/posts/{_}"},
		},
		{
			name:   "no parametric segments",
			method: "GET",
			path:   "/api/users",
			want:   Key{Method: "GET", Path: "/api/users"},
		},
		{
			name:   "empty path",
			method: "GET",
			path:   "",
			want:   Key{Method: "GET", Path: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewKey(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("NewKey(%q, %q) = %#v, want %#v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestKeyFromExchange(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{
			Method: "PATCH",
			Path:   "/api/orders/42/items/abc12345",
		},
	}
	got := KeyFromExchange(ex)
	want := Key{Method: "PATCH", Path: "/api/orders/{_}/items/{_}"}
	if got != want {
		t.Errorf("KeyFromExchange(%v) = %#v, want %#v", ex, got, want)
	}
}

func TestKey_String(t *testing.T) {
	tests := []struct {
		name string
		key  Key
		want string
	}{
		{
			name: "normalised path",
			key:  Key{Method: "GET", Path: "/api/users/{_}"},
			want: "GET|/api/users/{_}",
		},
		{
			name: "empty method",
			key:  Key{Method: "", Path: "/health"},
			want: "|/health",
		},
		{
			name: "empty path",
			key:  Key{Method: "GET", Path: ""},
			want: "GET|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.String(); got != tt.want {
				t.Errorf("Key.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKey_Equality(t *testing.T) {
	a := NewKey("GET", "/api/users/123")
	b := NewKey("GET", "/api/users/456")
	c := NewKey("POST", "/api/users/123")

	if a != b {
		t.Errorf("expected normalised keys to be equal: a=%v b=%v", a, b)
	}
	if a == c {
		t.Errorf("expected keys with different methods to differ: a=%v c=%v", a, c)
	}
}
