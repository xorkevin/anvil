package starlarkengine

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/mitchellh/mapstructure"
	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/scriptengine"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/anvil/util/stackset"
)

type (
	universeLibBase struct {
		root fs.FS
		args map[string]any
	}
)

func (l universeLibBase) mod() []NativeFunc {
	return []NativeFunc{
		{
			Name: "getargs",
			Fn:   l.getargs,
		},
		{
			Name:   "sleep",
			Fn:     l.getenv,
			Params: []string{"ms"},
		},
		{
			Name:   "getenv",
			Fn:     l.getenv,
			Params: []string{"name"},
		},
		{
			Name:   "json_marshal",
			Fn:     l.jsonMarshal,
			Params: []string{"v"},
		},
		{
			Name:   "json_unmarshal",
			Fn:     l.jsonUnmarshal,
			Params: []string{"s"},
		},
		{
			Name:   "json_mergepatch",
			Fn:     l.jsonMergePatch,
			Params: []string{"a", "b"},
		},
		{
			Name:   "path_join",
			Fn:     l.pathJoin,
			Params: []string{"segments"},
		},
		{
			Name:   "readfile",
			Fn:     l.readfile,
			Params: []string{"name"},
		},
		{
			Name:   "readmodfile",
			Fn:     l.readmodfile,
			Params: []string{"name"},
		},
		{
			Name:   "writefile",
			Fn:     l.writefile,
			Params: []string{"name", "data"},
		},
		{
			Name:   "gotmpl",
			Fn:     l.gotmpl,
			Params: []string{"tmpl", "args"},
		},
	}
}

func (l universeLibBase) getargs(ctx context.Context, args []any) (any, error) {
	return l.args, nil
}

func (l universeLibBase) sleep(ctx context.Context, args []any) (any, error) {
	ms, ok := args[0].(int)
	if !ok {
		return nil, fmt.Errorf("%w: Sleep time must be int", scriptengine.ErrInvalidArgs)
	}
	if ms < 1 {
		return nil, fmt.Errorf("%w: Must sleep for a positive amount of time", scriptengine.ErrInvalidArgs)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("Context cancelled: %w", context.Cause(ctx))
	case <-time.After(time.Duration(ms) * time.Millisecond):
	}
	return nil, nil
}

func (l universeLibBase) getenv(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Env name must be string", scriptengine.ErrInvalidArgs)
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (l universeLibBase) jsonMarshal(ctx context.Context, args []any) (any, error) {
	b, err := kjson.Marshal(args[0])
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal json: %w", err)
	}
	return string(b), nil
}

func (l universeLibBase) jsonUnmarshal(ctx context.Context, args []any) (any, error) {
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: JSON must be string", scriptengine.ErrInvalidArgs)
	}
	if s == "" {
		return nil, fmt.Errorf("%w: Empty json string", scriptengine.ErrInvalidArgs)
	}
	var v any
	if err := kjson.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal json: %w", err)
	}
	return v, nil
}

func (l universeLibBase) jsonMergePatch(ctx context.Context, args []any) (any, error) {
	return kjson.MergePatch(args[0], args[1]), nil
}

func (l universeLibBase) pathJoin(ctx context.Context, args []any) (any, error) {
	var segments []string
	if err := mapstructure.Decode(args[0], &segments); err != nil {
		return nil, fmt.Errorf("%w: Path segments must be an array of strings: %w", confengine.ErrInvalidArgs, err)
	}
	return path.Join(segments...), nil
}

func (l universeLibBase) readfile(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File name must be a string", scriptengine.ErrInvalidArgs)
	}
	b, err := os.ReadFile(filepath.FromSlash(name))
	if err != nil {
		return nil, fmt.Errorf("Failed reading file %s: %w", name, err)
	}
	return string(b), nil
}

func (l universeLibBase) readmodfile(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File name must be a string", scriptengine.ErrInvalidArgs)
	}
	b, err := fs.ReadFile(l.root, name)
	if err != nil {
		return nil, fmt.Errorf("Failed reading mod file %s: %w", name, err)
	}
	return string(b), nil
}

func (l universeLibBase) writefile(ctx context.Context, args []any) (any, error) {
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File name must be a string", scriptengine.ErrInvalidArgs)
	}
	data, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("%w: File data must be a string", scriptengine.ErrInvalidArgs)
	}
	if err := os.WriteFile(filepath.FromSlash(name), []byte(data), 0o644); err != nil {
		return nil, fmt.Errorf("Failed writing file %s: %w", name, err)
	}
	return nil, nil
}

func (l universeLibBase) gotmpl(ctx context.Context, args []any) (any, error) {
	tmpl, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: Template must be a string", scriptengine.ErrInvalidArgs)
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
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
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
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
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

	vaultCfg struct {
		addr      string
		token     string
		kubemount string
		kvmount   string
		dbmount   string
	}
)

func (l universeLibVault) mod() starlark.StringDict {
	return starlark.StringDict{
		"authkube":    starlark.NewBuiltin("authkube", l.authkube),
		"kvput":       starlark.NewBuiltin("kvput", l.kvput),
		"kvget":       starlark.NewBuiltin("kvget", l.kvget),
		"dbconfigput": starlark.NewBuiltin("dbconfigput", l.dbconfigput),
		"dbroleput":   starlark.NewBuiltin("dbroleput", l.dbroleput),
	}
}

func (l universeLibVault) readVaultCfg(vaultcfg *starlark.Dict) (*vaultCfg, error) {
	if vaultcfg == nil {
		return nil, fmt.Errorf("%w: Missing vault cfg", scriptengine.ErrInvalidArgs)
	}

	cfg := &vaultCfg{}
	if saddr, ok, err := vaultcfg.Get(starlark.String("addr")); err != nil {
		return nil, fmt.Errorf("Failed getting vault addr: %w", err)
	} else if !ok {
		return nil, fmt.Errorf("%w: Missing vault addr", scriptengine.ErrInvalidArgs)
	} else {
		addr, ok := starlark.AsString(saddr)
		if !ok || addr == "" {
			return nil, fmt.Errorf("%w: Invalid vault addr", scriptengine.ErrInvalidArgs)
		}
		cfg.addr = addr
	}

	if stoken, ok, err := vaultcfg.Get(starlark.String("token")); err != nil {
		return nil, fmt.Errorf("Failed getting vault token: %w", err)
	} else if ok {
		token, ok := starlark.AsString(stoken)
		if !ok || token == "" {
			return nil, fmt.Errorf("%w: Invalid vault token", scriptengine.ErrInvalidArgs)
		}
		cfg.token = token
	}

	if skubemount, ok, err := vaultcfg.Get(starlark.String("kubemount")); err != nil {
		return nil, fmt.Errorf("Failed getting vault kube mount: %w", err)
	} else if ok {
		kubemount, ok := starlark.AsString(skubemount)
		if !ok || kubemount == "" {
			return nil, fmt.Errorf("%w: Invalid vault kube mount", scriptengine.ErrInvalidArgs)
		}
		cfg.kubemount = kubemount
	}

	if skvmount, ok, err := vaultcfg.Get(starlark.String("kvmount")); err != nil {
		return nil, fmt.Errorf("Failed getting vault kv mount: %w", err)
	} else if ok {
		kvmount, ok := starlark.AsString(skvmount)
		if !ok || kvmount == "" {
			return nil, fmt.Errorf("%w: Invalid vault kv mount", scriptengine.ErrInvalidArgs)
		}
		cfg.kvmount = kvmount
	}

	if sdbmount, ok, err := vaultcfg.Get(starlark.String("dbmount")); err != nil {
		return nil, fmt.Errorf("Failed getting vault db mount: %w", err)
	} else if ok {
		dbmount, ok := starlark.AsString(sdbmount)
		if !ok || dbmount == "" {
			return nil, fmt.Errorf("%w: Invalid vault db mount", scriptengine.ErrInvalidArgs)
		}
		cfg.dbmount = dbmount
	}

	return cfg, nil
}

func (l universeLibVault) authkube(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}
	var role string
	var vaultcfg *starlark.Dict
	satokenfile := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	if err := starlark.UnpackArgs("authkube", args, kwargs, "role", &role, "vaultcfg", &vaultcfg, "satokenfile?", &satokenfile); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if role == "" {
		return nil, fmt.Errorf("%w: Empty role", scriptengine.ErrInvalidArgs)
	}

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.kubemount == "" {
		return nil, fmt.Errorf("%w: Invalid vault kv mount", scriptengine.ErrInvalidArgs)
	}
	satokenb, err := os.ReadFile(satokenfile)
	if err != nil {
		return nil, fmt.Errorf("Failed reading kube service account token file %s: %w", satokenfile, err)
	}
	if len(satokenb) == 0 {
		return nil, fmt.Errorf("Empty service account token file: %s", satokenfile)
	}
	satoken := string(satokenb)
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/v1/auth/%s/login", cfg.addr, cfg.kubemount), struct {
		JWT  string `json:"jwt"`
		Role string `json:"role"`
	}{
		JWT:  satoken,
		Role: role,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed creating vault kube auth request: %w", err)
	}
	retRes := starlark.NewDict(3)
	var resbody struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	res, _, err := l.httpClient.DoJSON(ctx, req, &resbody)
	if err != nil {
		if res != nil {
			retRes.SetKey(starlark.String("status"), starlark.MakeInt(res.StatusCode))
		}
		retRes.SetKey(starlark.String("error"), starlark.String(fmt.Errorf("Failed making vault kube auth request: %w", err).Error()))
		return retRes, nil
	}
	retRes.SetKey(starlark.String("status"), starlark.MakeInt(res.StatusCode))
	if resbody.Auth.ClientToken == "" {
		retRes.SetKey(starlark.String("error"), starlark.String("No vault client token"))
		return retRes, nil
	}
	retData := starlark.NewDict(1)
	retData.SetKey(starlark.String("token"), starlark.String(resbody.Auth.ClientToken))
	retRes.SetKey(starlark.String("data"), retData)
	return retRes, nil
}

func (l universeLibVault) doVaultReq(ctx context.Context, name string, cfg *vaultCfg, method string, path string, body any, retRes *starlark.Dict, resbody any) (bool, error) {
	if cfg.token == "" {
		return true, fmt.Errorf("%w: Missing vault token", scriptengine.ErrInvalidArgs)
	}
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/%s", cfg.addr, path), body)
	if err != nil {
		return true, fmt.Errorf("Failed creating vault %s request: %w", name, err)
	}
	req.Header.Set("X-Vault-Token", cfg.token)
	res, decoded, err := l.httpClient.DoJSON(ctx, req, resbody)
	if err != nil {
		if res != nil {
			retRes.SetKey(starlark.String("status"), starlark.MakeInt(res.StatusCode))
		}
		err := fmt.Errorf("Failed making vault %s request: %w", name, err)
		retRes.SetKey(starlark.String("error"), starlark.String(err.Error()))
		return false, err
	}
	retRes.SetKey(starlark.String("status"), starlark.MakeInt(res.StatusCode))
	if !decoded {
		err := fmt.Errorf("No vault %s response", name)
		retRes.SetKey(starlark.String("error"), starlark.String(err.Error()))
		return false, err
	}
	return false, nil
}

func (l universeLibVault) kvput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}
	var key string
	var value starlark.Value
	var vaultcfg *starlark.Dict
	cas := -1
	if err := starlark.UnpackArgs("kvput", args, kwargs, "key", &key, "value", &value, "vaultcfg", &vaultcfg, "cas?", &cas); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if key == "" {
		return nil, fmt.Errorf("%w: Empty key", scriptengine.ErrInvalidArgs)
	}
	if value == nil {
		return nil, fmt.Errorf("%w: Empty value", scriptengine.ErrInvalidArgs)
	}

	gvalue, err := starlarkToGoValue(value, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting vault kv value: %w", err)
	}

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.kvmount == "" {
		return nil, fmt.Errorf("%w: Missing vault kv mount", scriptengine.ErrInvalidArgs)
	}
	body := struct {
		Data    any `json:"data"`
		Options struct {
			CAS *int `json:"cas,omitempty"`
		} `json:"options"`
	}{
		Data: gvalue,
	}
	if cas >= 0 {
		body.Options.CAS = &cas
	}
	retRes := starlark.NewDict(3)
	var resbody struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if isFatal, err := l.doVaultReq(ctx, "kv put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/data/%s", cfg.kvmount, key), body, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	if resbody.Data.Version < 1 {
		retRes.SetKey(starlark.String("error"), starlark.String("No vault secret version"))
		return retRes, nil
	}
	retData := starlark.NewDict(1)
	retData.SetKey(starlark.String("version"), starlark.MakeInt(resbody.Data.Version))
	retRes.SetKey(starlark.String("data"), retData)
	return retRes, nil
}

func (l universeLibVault) kvget(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}
	var key string
	var vaultcfg *starlark.Dict
	if err := starlark.UnpackArgs("kvget", args, kwargs, "key", &key, "vaultcfg", &vaultcfg); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if key == "" {
		return nil, fmt.Errorf("%w: Empty key", scriptengine.ErrInvalidArgs)
	}

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.kvmount == "" {
		return nil, fmt.Errorf("%w: Missing vault kv mount", scriptengine.ErrInvalidArgs)
	}
	retRes := starlark.NewDict(3)
	var resbody struct {
		Data struct {
			Data     any `json:"data"`
			Metadata struct {
				Version int `json:"version"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if isFatal, err := l.doVaultReq(ctx, "kv get", cfg, http.MethodGet, fmt.Sprintf("v1/%s/data/%s", cfg.kvmount, key), nil, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	data, err := goToStarlarkValue(resbody.Data.Data, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting vault kv response: %w", err)
	}
	retData := starlark.NewDict(2)
	retData.SetKey(starlark.String("version"), starlark.MakeInt(resbody.Data.Metadata.Version))
	retData.SetKey(starlark.String("data"), data)
	retRes.SetKey(starlark.String("data"), retData)
	return retRes, nil
}

func (l universeLibVault) dbconfigput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}
	var name string
	var dbcfg *starlark.Dict
	var vaultcfg *starlark.Dict
	if err := starlark.UnpackArgs("dbconfigput", args, kwargs, "name", &name, "dbcfg", &dbcfg, "vaultcfg", &vaultcfg); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: Empty name", scriptengine.ErrInvalidArgs)
	}
	if dbcfg == nil {
		return nil, fmt.Errorf("%w: Missing db cfg", scriptengine.ErrInvalidArgs)
	}

	body, err := starlarkToGoValue(dbcfg, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting vault db cfg value: %w", err)
	}

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.dbmount == "" {
		return nil, fmt.Errorf("%w: Missing vault db mount", scriptengine.ErrInvalidArgs)
	}
	retRes := starlark.NewDict(3)
	var resbody any
	if isFatal, err := l.doVaultReq(ctx, "db cfg put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/config/%s", cfg.dbmount, name), body, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	return retRes, nil
}

func (l universeLibVault) dbroleput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}
	var name string
	var role *starlark.Dict
	var vaultcfg *starlark.Dict
	if err := starlark.UnpackArgs("dbroleput", args, kwargs, "name", &name, "role", &role, "vaultcfg", &vaultcfg); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: Empty name", scriptengine.ErrInvalidArgs)
	}
	if role == nil {
		return nil, fmt.Errorf("%w: Missing db role", scriptengine.ErrInvalidArgs)
	}

	body, err := starlarkToGoValue(role, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting vault db role value: %w", err)
	}

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.dbmount == "" {
		return nil, fmt.Errorf("%w: Missing vault db mount", scriptengine.ErrInvalidArgs)
	}
	retRes := starlark.NewDict(3)
	var resbody any
	if isFatal, err := l.doVaultReq(ctx, "db role put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/roles/%s", cfg.dbmount, name), body, retRes, &resbody); err != nil {
		if isFatal {
			return nil, err
		}
		return retRes, nil
	}
	return retRes, nil
}
