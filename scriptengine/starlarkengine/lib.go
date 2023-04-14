package starlarkengine

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/util/kjson"
)

type (
	universeLibBase struct{}
)

func (l universeLibBase) mod() starlark.StringDict {
	return starlark.StringDict{
		"getenv": starlark.NewBuiltin("getenv", l.getenv),
	}
}

func (l universeLibBase) getenv(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("genpass", args, kwargs, "name", &name); err != nil {
		return nil, fmt.Errorf("Invalid args: %w", err)
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return starlark.None, nil
	}
	return starlark.String(v), nil
}

type (
	universeLibCrypto struct{}
)

func (l universeLibCrypto) mod() starlark.StringDict {
	return starlark.StringDict{
		"genpass": starlark.NewBuiltin("genpass", l.genpass),
		"genrsa":  starlark.NewBuiltin("genrsa", l.genrsa),
	}
}

func (l universeLibCrypto) genpass(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var n uint = 64
	if err := starlark.UnpackArgs("genpass", args, kwargs, "n", &n); err != nil {
		return nil, fmt.Errorf("Invalid args: %w", err)
	}
	if n == 0 {
		return starlark.String(""), nil
	}
	b := make([]byte, n)
	if _, err := rand.Reader.Read(b); err != nil {
		return nil, fmt.Errorf("Failed reading rand bytes: %w", err)
	}
	return starlark.String(base64.RawURLEncoding.EncodeToString(b)), nil
}

const (
	pemBlockTypePrivateKey = "PRIVATE KEY"
	pemBlockTypePublicKey  = "PUBLIC KEY"
)

func (l universeLibCrypto) genrsa(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var n uint = 4096
	var blocktype string = pemBlockTypePrivateKey
	if err := starlark.UnpackArgs("genrsa", args, kwargs, "n", &n, "blocktype?", &blocktype); err != nil {
		return nil, fmt.Errorf("Invalid args: %w", err)
	}
	key, err := rsa.GenerateKey(rand.Reader, int(n))
	if err != nil {
		return nil, fmt.Errorf("Failed to generate rsa key: %w", err)
	}
	rawKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal rsa key: %w", err)
	}
	return starlark.String(pem.EncodeToMemory(&pem.Block{
		Type:  blocktype,
		Bytes: rawKey,
	})), nil
}

type (
	universeLibVault struct {
		httpClient *httpClient
	}
)

func (l universeLibVault) mod() starlark.StringDict {
	return starlark.StringDict{
		"authkube": starlark.NewBuiltin("authkube", l.authkube),
	}
}

func (l universeLibVault) authkube(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var role string
	var vaultcfg *starlark.Dict
	satokenfile := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	if err := starlark.UnpackArgs("authkube", args, kwargs, "role", &role, "vaultcfg", &vaultcfg, "satokenfile?", &satokenfile); err != nil {
		return nil, fmt.Errorf("Invalid args: %w", err)
	}
	if role == "" {
		return nil, errors.New("Empty role")
	}
	if vaultcfg == nil {
		return nil, errors.New("Missing vault cfg")
	}
	svaultaddr, ok, err := vaultcfg.Get(starlark.String("addr"))
	if err != nil {
		return nil, fmt.Errorf("Failed getting vault addr: %w", err)
	}
	if !ok {
		return nil, errors.New("Missing vault addr")
	}
	vaultaddr, ok := starlark.AsString(svaultaddr)
	if !ok || vaultaddr == "" {
		return nil, errors.New("Invalid vault addr")
	}
	svaultmount, ok, err := vaultcfg.Get(starlark.String("kubemount"))
	if err != nil {
		return nil, fmt.Errorf("Failed getting vault kube mount: %w", err)
	}
	if !ok {
		return nil, errors.New("Missing vault kube mount")
	}
	vaultmount, ok := starlark.AsString(svaultmount)
	if !ok || vaultmount == "" {
		return nil, errors.New("Invalid vault kube mount")
	}
	satokenb, err := os.ReadFile(satokenfile)
	if err != nil {
		return nil, fmt.Errorf("Failed reading kube service account token file %s: %w", satokenfile, err)
	}
	if len(satokenb) == 0 {
		return nil, fmt.Errorf("Empty service account token file: %s", satokenfile)
	}
	satoken := string(satokenb)
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/v1/auth/%s/login", vaultaddr, vaultmount), struct {
		JWT  string `json:"jwt"`
		Role string `json:"role"`
	}{
		JWT:  satoken,
		Role: role,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed creating vault kube auth request: %w", err)
	}
	var res struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	_, _, err = l.httpClient.DoJSON(context.Background(), req, &res)
	if err != nil {
		return nil, fmt.Errorf("Failed making vault kube auth request: %w", err)
	}
	if res.Auth.ClientToken == "" {
		return nil, errors.New("No vault client token")
	}
	return starlark.String(res.Auth.ClientToken), nil
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
		return nil, fmt.Errorf("Malformed http request: %w", err)
	}
	return req, nil
}

func (c *httpClient) Do(ctx context.Context, r *http.Request) (_ *http.Response, retErr error) {
	res, err := c.httpc.Do(r)
	if err != nil {
		return nil, fmt.Errorf("Failed request: %w", err)
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
			return res, fmt.Errorf("Failed reading error response: %w", err)
		}
		return res, fmt.Errorf("Received error response: %s", s.String())
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
			return res, false, fmt.Errorf("Failed decoding http response: %w", err)
		}
		decoded = true
	}
	return res, decoded, nil
}

func isHTTPStatusDecodable(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices && status != http.StatusNoContent
}
