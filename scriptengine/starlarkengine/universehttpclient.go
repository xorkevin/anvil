package starlarkengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xorkevin.dev/anvil/util/kjson"
)

var (
	// ErrInvalidClientReq is returned when the client request could not be made
	ErrInvalidClientReq errInvalidClientReq
	// ErrSendClientReq is returned when failing to send the client request
	ErrSendClientReq errSendClientReq
	// ErrInvalidServerRes is returned on an invalid server response
	ErrInvalidServerRes errInvalidServerRes
	// ErrServerRes is a returned server error
	ErrServerRes errServerRes
)

type (
	errInvalidClientReq struct{}
	errSendClientReq    struct{}
	errInvalidServerRes struct{}
	errServerRes        struct{}
)

func (e errInvalidClientReq) Error() string {
	return "Invalid client request"
}

func (e errSendClientReq) Error() string {
	return "Failed sending client request"
}

func (e errInvalidServerRes) Error() string {
	return "Invalid server response"
}

func (e errServerRes) Error() string {
	return "Error server response"
}

type (
	httpClient struct {
		httpc *http.Client
	}

	configHTTPClient struct {
		timeout   time.Duration
		transport http.RoundTripper
	}
)

func newHTTPClient(c configHTTPClient) *httpClient {
	return &httpClient{
		httpc: &http.Client{
			Transport: c.transport,
			Timeout:   c.timeout,
		},
	}
}

func (c *httpClient) Req(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClientReq, err)
	}
	return req, nil
}

func (c *httpClient) Do(ctx context.Context, r *http.Request) (_ *http.Response, retErr error) {
	res, err := c.httpc.Do(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSendClientReq, err)
	}
	if res.StatusCode >= http.StatusBadRequest {
		defer func() {
			if err := res.Body.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("Failed to close http response body: %w", err))
			}
		}()
		defer func() {
			if _, err := io.Copy(io.Discard, res.Body); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("Failed to discard http response body: %w", err))
			}
		}()
		var s strings.Builder
		_, err := io.Copy(&s, res.Body)
		if err != nil {
			return res, fmt.Errorf("%w: Failed reading response: %w", ErrInvalidServerRes, err)
		}
		return res, fmt.Errorf("%w: %s", ErrServerRes, s.String())
	}
	return res, nil
}

func (c *httpClient) ReqJSON(method, path string, data interface{}) (*http.Request, error) {
	b, err := kjson.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to encode body to json: %w", err)
	}
	body := bytes.NewReader(b)
	req, err := c.Req(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *httpClient) DoJSON(ctx context.Context, r *http.Request, response interface{}) (_ *http.Response, _ bool, retErr error) {
	res, err := c.Do(ctx, r)
	if err != nil {
		return res, false, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to close http response body: %w", err))
		}
	}()
	defer func() {
		if _, err := io.Copy(io.Discard, res.Body); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("Failed to discard http response body: %w", err))
		}
	}()

	decoded := false
	if response != nil && isHTTPStatusDecodable(res.StatusCode) {
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return res, false, fmt.Errorf("%w: Failed decoding response: %w", ErrInvalidServerRes, err)
		}
		decoded = true
	}
	return res, decoded, nil
}

func isHTTPStatusDecodable(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices && status != http.StatusNoContent
}
