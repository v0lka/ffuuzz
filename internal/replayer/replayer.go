package replayer

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"

	"ffuuzz/internal/recorder"
)

type Replayer struct {
	Client *http.Client
}

func New(client *http.Client) *Replayer {
	if client == nil {
		client = http.DefaultClient
	}
	return &Replayer{Client: client}
}

func (r *Replayer) Replay(ctx context.Context, tx recorder.TxRecord) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, tx.Method, tx.URL, nil)
	if err != nil {
		return nil, err
	}

	if tx.ReqBody != "" {
		bodyBytes, err := base64.StdEncoding.DecodeString(tx.ReqBody)
		if err == nil {
			req.Body = ioNopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
		}
	}

	for k, vv := range tx.ReqHeaders {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	return r.Client.Do(req)
}

type nopCloser struct {
	*bytes.Reader
}

func (n nopCloser) Close() error { return nil }

func ioNopCloser(r *bytes.Reader) nopCloser {
	return nopCloser{r}
}
