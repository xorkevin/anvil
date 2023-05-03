package starlarkengine

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/mitchellh/mapstructure"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/anvil/util/ktime"
	"xorkevin.dev/anvil/workflowengine"
)

type (
	universeLibBase struct {
		root       fs.FS
		args       map[string]any
		httpClient *httpClient
	}
)

func (l universeLibBase) mod() []NativeFunc {
	return []NativeFunc{
		{
			Name: "getargs",
			Fn:   l.getargs,
		},
		{
			Mod:    "time",
			Name:   "sleep",
			Fn:     l.sleep,
			Params: []string{"ms"},
		},
		{
			Mod:    "json",
			Name:   "marshal",
			Fn:     l.jsonMarshal,
			Params: []string{"v"},
		},
		{
			Mod:    "json",
			Name:   "unmarshal",
			Fn:     l.jsonUnmarshal,
			Params: []string{"s"},
		},
		{
			Mod:    "json",
			Name:   "mergepatch",
			Fn:     l.jsonMergePatch,
			Params: []string{"a", "b"},
		},
		{
			Mod:    "yaml",
			Name:   "marshal",
			Fn:     l.yamlMarshal,
			Params: []string{"v"},
		},
		{
			Mod:    "yaml",
			Name:   "unmarshal",
			Fn:     l.yamlUnmarshal,
			Params: []string{"v"},
		},
		{
			Mod:    "path",
			Name:   "join",
			Fn:     l.pathJoin,
			Params: []string{"segments"},
		},
		{
			Mod:    "os",
			Name:   "getenv",
			Fn:     l.getenv,
			Params: []string{"name"},
		},
		{
			Mod:    "os",
			Name:   "readfile",
			Fn:     l.readfile,
			Params: []string{"name"},
		},
		{
			Mod:    "os",
			Name:   "readmodfile",
			Fn:     l.readmodfile,
			Params: []string{"name"},
		},
		{
			Mod:    "os",
			Name:   "writefile",
			Fn:     l.writefile,
			Params: []string{"name", "data"},
		},
		{
			Mod:    "http",
			Name:   "doreq",
			Fn:     l.httpDoReq,
			Params: []string{"method", "url", "opts", "body"},
		},
		{
			Mod:    "template",
			Name:   "gotpl",
			Fn:     l.gotpl,
			Params: []string{"tpl", "args"},
		},
		{
			Mod:    "crypto",
			Name:   "genpass",
			Fn:     l.genpass,
			Params: []string{"n"},
		},
		{
			Mod:    "crypto",
			Name:   "genrsa",
			Fn:     l.genrsa,
			Params: []string{"n", "blocktype?"},
		},
		{
			Mod:    "crypto",
			Name:   "sha256hex",
			Fn:     l.sha256hex,
			Params: []string{"data"},
		},
		{
			Mod:    "crypto",
			Name:   "bcrypt",
			Fn:     l.bcrypt,
			Params: []string{"password", "cost"},
		},
		{
			Mod:    "vault",
			Name:   "authkube",
			Fn:     l.authkube,
			Params: []string{"role", "vaultcfg", "satokenfile?"},
		},
		{
			Mod:    "vault",
			Name:   "kvput",
			Fn:     l.kvput,
			Params: []string{"key", "value", "vaultcfg", "cas?"},
		},
		{
			Mod:    "vault",
			Name:   "kvget",
			Fn:     l.kvget,
			Params: []string{"key", "vaultcfg"},
		},
		{
			Mod:    "vault",
			Name:   "dbconfigput",
			Fn:     l.dbconfigput,
			Params: []string{"name", "dbcfg", "vaultcfg"},
		},
		{
			Mod:    "vault",
			Name:   "dbroleput",
			Fn:     l.dbroleput,
			Params: []string{"name", "rolecfg", "vaultcfg"},
		},
	}
}

func (l *universeLibBase) getargs(ctx context.Context, args []any) (any, error) {
	return l.args, nil
}

func (l *universeLibBase) sleep(ctx context.Context, args []any) (any, error) {
	ms, ok := args[0].(int)
	if !ok {
		return nil, fmt.Errorf("%w: Sleep time must be int", workflowengine.ErrInvalidArgs)
	}
	if ms < 1 {
		return nil, fmt.Errorf("%w: Must sleep for a positive amount of time", workflowengine.ErrInvalidArgs)
	}
	if err := ktime.After(ctx, time.Duration(ms)*time.Millisecond); err != nil {
		return nil, err
	}
	return nil, nil
}

func (l *universeLibBase) getenv(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Env name must be a string", workflowengine.ErrInvalidArgs)
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (l *universeLibBase) jsonMarshal(ctx context.Context, args []any) (any, error) {
	b, err := kjson.Marshal(args[0])
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal json: %w", err)
	}
	return string(b), nil
}

func (l *universeLibBase) jsonUnmarshal(ctx context.Context, args []any) (any, error) {
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: JSON must be a string", workflowengine.ErrInvalidArgs)
	}
	if s == "" {
		return nil, fmt.Errorf("%w: Empty json string", workflowengine.ErrInvalidArgs)
	}
	var v any
	if err := kjson.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal json: %w", err)
	}
	return v, nil
}

func (l *universeLibBase) jsonMergePatch(ctx context.Context, args []any) (any, error) {
	return kjson.MergePatch(args[0], args[1]), nil
}

func (l *universeLibBase) yamlMarshal(ctx context.Context, args []any) (any, error) {
	b, err := yaml.Marshal(args[0])
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal yaml: %w", err)
	}
	return string(b), nil
}

func (l *universeLibBase) yamlUnmarshal(ctx context.Context, args []any) (any, error) {
	b, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: YAML must be a string", workflowengine.ErrInvalidArgs)
	}
	var v any
	if err := yaml.Unmarshal([]byte(b), &v); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal yaml: %w", err)
	}
	return v, nil
}

func (l *universeLibBase) pathJoin(ctx context.Context, args []any) (any, error) {
	var segments []string
	if err := mapstructure.Decode(args[0], &segments); err != nil {
		return nil, fmt.Errorf("%w: Path segments must be an array of strings: %w", confengine.ErrInvalidArgs, err)
	}
	return path.Join(segments...), nil
}

func (l *universeLibBase) readfile(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File name must be a string", workflowengine.ErrInvalidArgs)
	}
	b, err := os.ReadFile(filepath.FromSlash(name))
	if err != nil {
		return nil, fmt.Errorf("Failed reading file %s: %w", name, err)
	}
	return string(b), nil
}

func (l *universeLibBase) readmodfile(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File name must be a string", workflowengine.ErrInvalidArgs)
	}
	b, err := fs.ReadFile(l.root, name)
	if err != nil {
		return nil, fmt.Errorf("Failed reading mod file %s: %w", name, err)
	}
	return string(b), nil
}

func (l *universeLibBase) writefile(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File name must be a string", workflowengine.ErrInvalidArgs)
	}
	data, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File data must be a string", workflowengine.ErrInvalidArgs)
	}
	if err := os.WriteFile(filepath.FromSlash(name), []byte(data), 0o644); err != nil {
		return nil, fmt.Errorf("Failed writing file %s: %w", name, err)
	}
	return nil, nil
}

type (
	httpReqOpts struct {
		BasicAuth struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"basicauth"`
		Header  map[string]any `json:"header"`
		JSONReq bool           `json:"jsonreq"`
		JSONRes bool           `json:"jsonres"`
	}
)

func (l *universeLibBase) httpDoReq(ctx context.Context, args []any) (_ any, retErr error) {
	method, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Method must be a string", workflowengine.ErrInvalidArgs)
	}
	url, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("%w: URL must be a string", workflowengine.ErrInvalidArgs)
	}
	var opts httpReqOpts
	if err := mapstructure.Decode(args[2], &opts); err != nil {
		return nil, fmt.Errorf("%w: Invalid http req opts: %w", confengine.ErrInvalidArgs, err)
	}
	var req *http.Request
	if opts.JSONReq {
		var err error
		req, err = l.httpClient.ReqJSON(method, url, args[3])
		if err != nil {
			return nil, fmt.Errorf("%w: Failed to construct http json request: %w", confengine.ErrInvalidArgs, err)
		}
	} else {
		body, ok := args[3].(string)
		if !ok {
			return nil, fmt.Errorf("%w: Body must be a string", workflowengine.ErrInvalidArgs)
		}
		var err error
		req, err = l.httpClient.Req(method, url, strings.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("%w: Failed to construct http request: %w", confengine.ErrInvalidArgs, err)
		}
	}
	for k, v := range opts.Header {
		val, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: Header value must be a string", confengine.ErrInvalidArgs)
		}
		req.Header.Set(k, val)
	}
	if opts.BasicAuth.Username != "" {
		req.SetBasicAuth(opts.BasicAuth.Username, opts.BasicAuth.Password)
	}
	if opts.JSONRes {
		retRes := map[string]any{}
		var resbody any
		res, decoded, err := l.httpClient.DoJSON(ctx, req, &resbody)
		if err != nil {
			if res != nil {
				retRes["status"] = res.StatusCode
			}
			retRes["error"] = fmt.Errorf("Failed making http request: %w", err).Error()
			return retRes, nil
		}
		retRes["status"] = res.StatusCode
		if !decoded {
			retRes["error"] = "No http response body"
			return retRes, nil
		}
		retRes["data"] = resbody
		return retRes, nil
	}
	retRes := map[string]any{}
	res, resbody, err := l.httpClient.DoStr(ctx, req)
	if err != nil {
		if res != nil {
			retRes["status"] = res.StatusCode
		}
		retRes["error"] = fmt.Errorf("Failed making http request: %w", err).Error()
		return retRes, nil
	}
	retRes["status"] = res.StatusCode
	retRes["data"] = resbody
	return retRes, nil
}

func (l *universeLibBase) gotpl(ctx context.Context, args []any) (any, error) {
	tmpl, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Template must be a string", workflowengine.ErrInvalidArgs)
	}
	t, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("Failed parsing template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, args[1]); err != nil {
		return nil, fmt.Errorf("Failed executing template: %w", err)
	}
	return b.String(), nil
}

func (l *universeLibBase) genpass(ctx context.Context, args []any) (any, error) {
	n, ok := args[0].(int)
	if !ok {
		return nil, fmt.Errorf("%w: Number of bytes must be an integer", workflowengine.ErrInvalidArgs)
	}
	if n < 1 {
		return nil, fmt.Errorf("%w: Number of bytes must be a positive integer", workflowengine.ErrInvalidArgs)
	}
	b := make([]byte, n)
	if _, err := rand.Reader.Read(b); err != nil {
		return nil, fmt.Errorf("Failed reading rand bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const (
	pemBlockTypePrivateKey = "PRIVATE KEY"
	pemBlockTypePublicKey  = "PUBLIC KEY"
)

func (l *universeLibBase) genrsa(ctx context.Context, args []any) (any, error) {
	n, ok := args[0].(int)
	if !ok {
		return nil, fmt.Errorf("%w: Key size must be an integer", workflowengine.ErrInvalidArgs)
	}
	blocktype := pemBlockTypePrivateKey
	if args[1] != nil {
		var ok bool
		blocktype, ok = args[1].(string)
		if !ok {
			return nil, fmt.Errorf("%w: PEM block type must be a string", workflowengine.ErrInvalidArgs)
		}
		if blocktype == "" {
			return nil, fmt.Errorf("%w: Empty PEM block type", workflowengine.ErrInvalidArgs)
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, n)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate rsa key: %w", err)
	}
	rawKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal rsa key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  blocktype,
		Bytes: rawKey,
	}), nil
}

func (l *universeLibBase) sha256hex(ctx context.Context, args []any) (any, error) {
	data, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Data must be a string", workflowengine.ErrInvalidArgs)
	}
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:]), nil
}

func (l *universeLibBase) bcrypt(ctx context.Context, args []any) (any, error) {
	password, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Password must be a string", workflowengine.ErrInvalidArgs)
	}
	cost, ok := args[1].(int)
	if !ok {
		return nil, fmt.Errorf("%w: Cost must be an integer", workflowengine.ErrInvalidArgs)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return nil, fmt.Errorf("Failed to hash password: %w", err)
	}
	return string(h), nil
}

type (
	vaultCfg struct {
		Addr      string `mapstructure:"addr"`
		Token     string `mapstructure:"token"`
		KubeMount string `mapstructure:"kubemount"`
		KVMount   string `mapstructure:"kvmount"`
		DBMount   string `mapstructure:"dbmount"`
	}
)

func (l *universeLibBase) readVaultCfg(vaultcfg any) (*vaultCfg, error) {
	cfg := &vaultCfg{}
	if err := mapstructure.Decode(vaultcfg, cfg); err != nil {
		return nil, fmt.Errorf("%w: Invalid vault cfg: %w", confengine.ErrInvalidArgs, err)
	}
	if cfg.Addr == "" {
		return nil, fmt.Errorf("%w: Missing vault addr", workflowengine.ErrInvalidArgs)
	}
	return cfg, nil
}

func (l *universeLibBase) authkube(ctx context.Context, args []any) (any, error) {
	role, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Kube role must be a string", workflowengine.ErrInvalidArgs)
	}
	if role == "" {
		return nil, fmt.Errorf("%w: Empty role", workflowengine.ErrInvalidArgs)
	}
	cfg, err := l.readVaultCfg(args[1])
	if err != nil {
		return nil, err
	}
	if cfg.KubeMount == "" {
		return nil, fmt.Errorf("%w: Invalid vault kube auth mount", workflowengine.ErrInvalidArgs)
	}
	satokenfile := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	if args[2] != nil {
		var ok bool
		satokenfile, ok = args[2].(string)
		if !ok {
			return nil, fmt.Errorf("%w: Service account token file must be a string", workflowengine.ErrInvalidArgs)
		}
	}

	satoken, err := os.ReadFile(filepath.FromSlash(satokenfile))
	if err != nil {
		return nil, fmt.Errorf("Failed reading kube service account token file %s: %w", satokenfile, err)
	}
	if len(satoken) == 0 {
		return nil, fmt.Errorf("Empty service account token file: %s", satokenfile)
	}
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/v1/auth/%s/login", cfg.Addr, cfg.KubeMount), struct {
		JWT  string `json:"jwt"`
		Role string `json:"role"`
	}{
		JWT:  string(satoken),
		Role: role,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed creating vault kube auth request: %w", err)
	}
	retRes := map[string]any{}
	var resbody struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	res, decoded, err := l.httpClient.DoJSON(ctx, req, &resbody)
	if err != nil {
		if res != nil {
			retRes["status"] = res.StatusCode
		}
		retRes["error"] = fmt.Errorf("Failed making vault kube auth request: %w", err).Error()
		return retRes, nil
	}
	retRes["status"] = res.StatusCode
	if !decoded {
		retRes["error"] = "No vault kube auth response"
		return retRes, nil
	}
	if resbody.Auth.ClientToken == "" {
		retRes["error"] = "No vault client token"
		return retRes, nil
	}
	retRes["data"] = map[string]any{
		"token": resbody.Auth.ClientToken,
	}
	return retRes, nil
}

func (l *universeLibBase) doVaultReq(ctx context.Context, name string, cfg *vaultCfg, method string, path string, body any, retRes map[string]any, resbody any) (bool, error) {
	if cfg.Token == "" {
		return true, fmt.Errorf("%w: Missing vault token", workflowengine.ErrInvalidArgs)
	}
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/%s", cfg.Addr, path), body)
	if err != nil {
		return true, fmt.Errorf("Failed creating vault %s request: %w", name, err)
	}
	req.Header.Set("X-Vault-Token", cfg.Token)
	res, decoded, err := l.httpClient.DoJSON(ctx, req, resbody)
	if err != nil {
		if res != nil {
			retRes["status"] = res.StatusCode
		}
		err := fmt.Errorf("Failed making vault %s request: %w", name, err)
		retRes["error"] = err.Error()
		return false, err
	}
	retRes["status"] = res.StatusCode
	if !decoded {
		err := fmt.Errorf("No vault %s response", name)
		retRes["error"] = err.Error()
		return false, err
	}
	return false, nil
}

func (l *universeLibBase) kvput(ctx context.Context, args []any) (any, error) {
	key, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Key must be a string", workflowengine.ErrInvalidArgs)
	}
	if key == "" {
		return nil, fmt.Errorf("%w: Empty key", workflowengine.ErrInvalidArgs)
	}
	value, ok := args[1].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: Value must be an object", workflowengine.ErrInvalidArgs)
	}
	cfg, err := l.readVaultCfg(args[2])
	if err != nil {
		return nil, err
	}
	if cfg.KVMount == "" {
		return nil, fmt.Errorf("%w: Missing vault kv mount", workflowengine.ErrInvalidArgs)
	}
	cas := -1
	if args[3] != nil {
		var ok bool
		cas, ok = args[3].(int)
		if !ok {
			return nil, fmt.Errorf("%w: CAS must be an integer", workflowengine.ErrInvalidArgs)
		}
	}
	body := struct {
		Data    any `json:"data"`
		Options struct {
			CAS *int `json:"cas,omitempty"`
		} `json:"options"`
	}{
		Data: value,
	}
	if cas >= 0 {
		body.Options.CAS = &cas
	}
	retRes := map[string]any{}
	var resbody struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if isFatal, err := l.doVaultReq(ctx, "kv put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/data/%s", cfg.KVMount, key), body, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	if resbody.Data.Version < 1 {
		retRes["error"] = "No vault secret version"
		return retRes, nil
	}
	retRes["data"] = map[string]any{
		"version": resbody.Data.Version,
	}
	return retRes, nil
}

func (l *universeLibBase) kvget(ctx context.Context, args []any) (any, error) {
	key, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Key must be a string", workflowengine.ErrInvalidArgs)
	}
	if key == "" {
		return nil, fmt.Errorf("%w: Empty key", workflowengine.ErrInvalidArgs)
	}
	cfg, err := l.readVaultCfg(args[1])
	if err != nil {
		return nil, err
	}
	if cfg.KVMount == "" {
		return nil, fmt.Errorf("%w: Missing vault kv mount", workflowengine.ErrInvalidArgs)
	}
	retRes := map[string]any{}
	var resbody struct {
		Data struct {
			Data     any `json:"data"`
			Metadata struct {
				Version int `json:"version"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if isFatal, err := l.doVaultReq(ctx, "kv get", cfg, http.MethodGet, fmt.Sprintf("v1/%s/data/%s", cfg.KVMount, key), nil, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	if resbody.Data.Metadata.Version < 1 {
		retRes["error"] = "No vault secret version"
		return retRes, nil
	}
	retRes["data"] = map[string]any{
		"data":    resbody.Data,
		"version": resbody.Data.Metadata.Version,
	}
	return retRes, nil
}

func (l *universeLibBase) dbconfigput(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Name must be a string", workflowengine.ErrInvalidArgs)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: Empty name", workflowengine.ErrInvalidArgs)
	}
	dbcfg, ok := args[1].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: DB config must be an object", workflowengine.ErrInvalidArgs)
	}
	cfg, err := l.readVaultCfg(args[2])
	if err != nil {
		return nil, err
	}
	if cfg.DBMount == "" {
		return nil, fmt.Errorf("%w: Missing vault db mount", workflowengine.ErrInvalidArgs)
	}
	retRes := map[string]any{}
	var resbody any
	if isFatal, err := l.doVaultReq(ctx, "db cfg put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/config/%s", cfg.DBMount, name), dbcfg, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	return retRes, nil
}

func (l *universeLibBase) dbroleput(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Name must be a string", workflowengine.ErrInvalidArgs)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: Empty name", workflowengine.ErrInvalidArgs)
	}
	rolecfg, ok := args[1].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: Role config must be an object", workflowengine.ErrInvalidArgs)
	}
	cfg, err := l.readVaultCfg(args[2])
	if err != nil {
		return nil, err
	}
	if cfg.DBMount == "" {
		return nil, fmt.Errorf("%w: Missing vault db mount", workflowengine.ErrInvalidArgs)
	}
	retRes := map[string]any{}
	var resbody any
	if isFatal, err := l.doVaultReq(ctx, "db role put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/roles/%s", cfg.DBMount, name), rolecfg, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	return retRes, nil
}
