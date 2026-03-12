package httputil

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NewRequestID generates a request ID in the format YYYYMMDD-UUIDv4.
func NewRequestID() string {
	return fmt.Sprintf("%s-%s", time.Now().Format("20060102"), uuid.New().String())
}
