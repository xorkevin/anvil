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
	"net/http"
	"os"

	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/util/stackset"
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

	vaultCfg struct {
		addr      string
		token     string
		kubemount string
		kvmount   string
	}
)

func (l universeLibVault) mod() starlark.StringDict {
	return starlark.StringDict{
		"authkube": starlark.NewBuiltin("authkube", l.authkube),
	}
}

func (l universeLibVault) readVaultCfg(vaultcfg *starlark.Dict) (*vaultCfg, error) {
	if vaultcfg == nil {
		return nil, errors.New("Missing vault cfg")
	}

	cfg := &vaultCfg{}
	if saddr, ok, err := vaultcfg.Get(starlark.String("addr")); err != nil {
		return nil, fmt.Errorf("Failed getting vault addr: %w", err)
	} else if !ok {
		return nil, errors.New("Missing vault addr")
	} else {
		addr, ok := starlark.AsString(saddr)
		if !ok || addr == "" {
			return nil, errors.New("Invalid vault addr")
		}
		cfg.addr = addr
	}

	if stoken, ok, err := vaultcfg.Get(starlark.String("token")); err != nil {
		return nil, fmt.Errorf("Failed getting vault token: %w", err)
	} else if ok {
		token, ok := starlark.AsString(stoken)
		if !ok || token == "" {
			return nil, errors.New("Invalid vault kube mount")
		}
		cfg.token = token
	}

	if skubemount, ok, err := vaultcfg.Get(starlark.String("kubemount")); err != nil {
		return nil, fmt.Errorf("Failed getting vault kube mount: %w", err)
	} else if ok {
		kubemount, ok := starlark.AsString(skubemount)
		if !ok || kubemount == "" {
			return nil, errors.New("Invalid vault kube mount")
		}
		cfg.kubemount = kubemount
	}

	if skvmount, ok, err := vaultcfg.Get(starlark.String("kvmount")); err != nil {
		return nil, fmt.Errorf("Failed getting vault kv mount: %w", err)
	} else if ok {
		kvmount, ok := starlark.AsString(skvmount)
		if !ok || kvmount == "" {
			return nil, errors.New("Invalid vault kv mount")
		}
		cfg.kvmount = kvmount
	}

	return cfg, nil
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

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.kubemount == "" {
		return nil, errors.New("Missing vault kube mount")
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

func (l universeLibVault) kvput(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	var value starlark.Value
	var vaultcfg *starlark.Dict
	cas := -1
	if err := starlark.UnpackArgs("authkube", args, kwargs, "key", &key, "value", &value, "vaultcfg", &vaultcfg, "cas?", &cas); err != nil {
		return nil, fmt.Errorf("Invalid args: %w", err)
	}
	if key == "" {
		return nil, errors.New("Empty key")
	}
	if value == nil {
		return nil, errors.New("Empty value")
	}

	gvalue, err := starlarkToGoValue(value, stackset.NewAny())
	if err != nil {
		return nil, err
	}

	cfg, err := l.readVaultCfg(vaultcfg)
	if err != nil {
		return nil, err
	}
	if cfg.token == "" {
		return nil, errors.New("Missing vault token")
	}
	if cfg.kvmount == "" {
		return nil, errors.New("Missing vault kv mount")
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
	req, err := l.httpClient.ReqJSON(http.MethodPost, fmt.Sprintf("%s/v1/%s/data/%s", cfg.addr, cfg.kvmount, key), body)
	if err != nil {
		return nil, fmt.Errorf("Failed creating vault kv put request: %w", err)
	}
	req.Header.Set("X-Vault-Token", cfg.token)
	var res struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	_, _, err = l.httpClient.DoJSON(context.Background(), req, &res)
	if err != nil {
		return nil, fmt.Errorf("Failed making vault kv put request: %w", err)
	}
	if res.Data.Version < 1 {
		return nil, errors.New("No vault secret version")
	}
	retData := starlark.NewDict(1)
	retData.SetKey(starlark.String("version"), starlark.MakeInt(res.Data.Version))
	return retData, nil
}
