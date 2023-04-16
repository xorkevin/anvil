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

	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/scriptengine"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/anvil/util/stackset"
)

type (
	universeLibBase struct {
		root fs.FS
		args *starlark.Dict
	}
)

func (l universeLibBase) mod() starlark.StringDict {
	return starlark.StringDict{
		"getargs":         starlark.NewBuiltin("getargs", l.getargs),
		"sleep":           starlark.NewBuiltin("sleep", l.getenv),
		"getenv":          starlark.NewBuiltin("getenv", l.getenv),
		"json_marshal":    starlark.NewBuiltin("json_marshal", l.jsonMarshal),
		"json_unmarshal":  starlark.NewBuiltin("json_unmarshal", l.jsonUnmarshal),
		"json_mergepatch": starlark.NewBuiltin("json_mergepatch", l.jsonMergePatch),
		"path_join":       starlark.NewBuiltin("path_join", l.pathJoin),
		"readfile":        starlark.NewBuiltin("readfile", l.readfile),
		"readmodfile":     starlark.NewBuiltin("readmodfile", l.readmodfile),
		"writefile":       starlark.NewBuiltin("writefile", l.writefile),
		"gotmpl":          starlark.NewBuiltin("gotmpl", l.gotmpl),
	}
}

func (l universeLibBase) getargs(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return l.args, nil
}

func (l universeLibBase) sleep(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ms int64
	if err := starlark.UnpackArgs("sleep", args, kwargs, "ms", &ms); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if ms < 1 {
		return nil, fmt.Errorf("%w: Must sleep for a positive amount of time", scriptengine.ErrInvalidArgs)
	}
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("Context cancelled: %w", context.Cause(ctx))
	case <-time.After(time.Duration(ms) * time.Millisecond):
	}
	return starlark.None, nil
}

func (l universeLibBase) getenv(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("getenv", args, kwargs, "name", &name); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return starlark.None, nil
	}
	return starlark.String(v), nil
}

func (l universeLibBase) jsonMarshal(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	if err := starlark.UnpackArgs("json_marshal", args, kwargs, "v", &v); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if v == nil {
		return nil, fmt.Errorf("%w: Empty value", scriptengine.ErrInvalidArgs)
	}

	gv, err := starlarkToGoValue(v, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting starlark value: %w", err)
	}
	b, err := kjson.Marshal(gv)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal json: %w", err)
	}
	return starlark.String(b), nil
}

func (l universeLibBase) jsonUnmarshal(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s string
	if err := starlark.UnpackArgs("json_unmarshal", args, kwargs, "s", &s); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if s == "" {
		return nil, fmt.Errorf("%w: Empty json string", scriptengine.ErrInvalidArgs)
	}

	var v any
	if err := kjson.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal json: %w", err)
	}

	sv, err := goToStarlarkValue(v, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting go value: %w", err)
	}
	return sv, nil
}

func (l universeLibBase) jsonMergePatch(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var a starlark.Value
	var b starlark.Value
	if err := starlark.UnpackArgs("json_marshal", args, kwargs, "a", &a, "b", &b); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if a == nil {
		return nil, fmt.Errorf("%w: Empty value", scriptengine.ErrInvalidArgs)
	}
	if b == nil {
		return nil, fmt.Errorf("%w: Empty value", scriptengine.ErrInvalidArgs)
	}

	ss := stackset.NewAny()
	ga, err := starlarkToGoValue(a, ss)
	if err != nil {
		return nil, fmt.Errorf("Failed converting starlark value: %w", err)
	}
	gb, err := starlarkToGoValue(b, ss)
	if err != nil {
		return nil, fmt.Errorf("Failed converting starlark value: %w", err)
	}
	v := kjson.MergePatch(ga, gb)
	sv, err := goToStarlarkValue(v, ss)
	if err != nil {
		return nil, fmt.Errorf("Failed converting go value: %w", err)
	}
	return sv, nil
}

func (l universeLibBase) pathJoin(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var segments *starlark.List
	if err := starlark.UnpackArgs("path_join", args, kwargs, "segments", &segments); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if segments == nil {
		segments = starlark.NewList(nil)
	}
	segs := make([]string, 0, segments.Len())
	for i := 0; i < segments.Len(); i++ {
		v := segments.Index(i)
		s, ok := v.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("%w: Path segment must be string", scriptengine.ErrInvalidArgs)
		}
		segs = append(segs, string(s))
	}
	return starlark.String(path.Join(segs...)), nil
}

func (l universeLibBase) readfile(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("readfile", args, kwargs, "name", &name); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	b, err := os.ReadFile(filepath.FromSlash(name))
	if err != nil {
		return nil, fmt.Errorf("Failed reading file %s: %w", name, err)
	}
	return starlark.String(b), nil
}

func (l universeLibBase) readmodfile(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("readmodfile", args, kwargs, "name", &name); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	b, err := fs.ReadFile(l.root, name)
	if err != nil {
		return nil, fmt.Errorf("Failed reading mod file %s: %w", name, err)
	}
	return starlark.String(b), nil
}

func (l universeLibBase) writefile(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var data string
	if err := starlark.UnpackArgs("writefile", args, kwargs, "name", &name, "data", &data); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if err := os.WriteFile(filepath.FromSlash(name), []byte(data), 0o644); err != nil {
		return nil, fmt.Errorf("Failed writing file %s: %w", name, err)
	}
	return starlark.None, nil
}

func (l universeLibBase) gotmpl(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var tmpl string
	var tmplargs *starlark.Dict
	if err := starlark.UnpackArgs("gotmpl", args, kwargs, "tmpl", &tmpl, "args", &tmplargs); err != nil {
		return nil, fmt.Errorf("%w: %w", scriptengine.ErrInvalidArgs, err)
	}
	if tmplargs == nil {
		tmplargs = starlark.NewDict(0)
	}
	gtmplargs, err := starlarkToGoValue(tmplargs, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting starlark value: %w", err)
	}
	tt, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("Failed parsing template: %w", err)
	}
	var b strings.Builder
	if err := tt.Execute(&b, gtmplargs); err != nil {
		return nil, fmt.Errorf("Failed executing template: %w", err)
	}
	return starlark.String(b.String()), nil
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

func (l universeLibVault) doVaultReq(name string, cfg *vaultCfg, method string, path string, body any, res any) error {
	if cfg.token == "" {
		return fmt.Errorf("%w: Missing vault token", scriptengine.ErrInvalidArgs)
	}
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/%s", cfg.addr, path), body)
	if err != nil {
		return fmt.Errorf("Failed creating vault %s request: %w", name, err)
	}
	req.Header.Set("X-Vault-Token", cfg.token)
	_, decoded, err := l.httpClient.DoJSON(context.Background(), req, res)
	if err != nil {
		return fmt.Errorf("Failed making vault %s request: %w", name, err)
	}
	if !decoded {
		return fmt.Errorf("No vault %s response", name)
	}
	return nil
}

func (l universeLibVault) kvput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
	var res struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if err := l.doVaultReq("kv put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/data/%s", cfg.kvmount, key), body, &res); err != nil {
		return nil, err
	}
	if res.Data.Version < 1 {
		return nil, errors.New("No vault secret version")
	}
	retData := starlark.NewDict(1)
	retData.SetKey(starlark.String("version"), starlark.MakeInt(res.Data.Version))
	return retData, nil
}

func (l universeLibVault) kvget(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
	var res struct {
		Data struct {
			Data     any `json:"data"`
			Metadata struct {
				Version int `json:"version"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := l.doVaultReq("kv get", cfg, http.MethodGet, fmt.Sprintf("v1/%s/data/%s", cfg.kvmount, key), nil, &res); err != nil {
		return nil, err
	}
	data, err := goToStarlarkValue(res.Data.Data, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed converting vault kv response: %w", err)
	}
	retData := starlark.NewDict(2)
	retData.SetKey(starlark.String("version"), starlark.MakeInt(res.Data.Metadata.Version))
	retData.SetKey(starlark.String("data"), data)
	return retData, nil
}

func (l universeLibVault) dbconfigput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
	var res any
	if err := l.doVaultReq("db cfg put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/config/%s", cfg.dbmount, name), body, &res); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (l universeLibVault) dbroleput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
	var res any
	if err := l.doVaultReq("db role put", cfg, http.MethodPost, fmt.Sprintf("v1/%s/roles/%s", cfg.dbmount, name), body, &res); err != nil {
		return nil, err
	}
	return starlark.None, nil
}
