package vault

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	roleKindKube = "kube"
)

// ErrInvalidRoleKind is returned when attempting to parse a role with an invalid kind
var ErrInvalidRoleKind errInvalidRoleKind

type (
	errInvalidRoleKind struct{}
)

func (e errInvalidRoleKind) Error() string {
	return "Invalid role kind"
}

type (
	// VaultClient interacts with vault
	VaultClient interface {
		PutPolicy(name, rules string) error
		PutRole(mount, name string, body map[string]interface{}) error
	}

	// HTTPVaultClient interacts with vault via its http api
	HTTPVaultClient struct {
		client  *vaultapi.Client
		sys     *vaultapi.Sys
		logical *vaultapi.Logical
	}
)

// NewHTTPVaultClient creates a new HTTPVaultClient
func NewHTTPVaultClient() (*HTTPVaultClient, error) {
	config := vaultapi.DefaultConfig()
	if err := config.Error; err != nil {
		return nil, kerrors.WithMsg(err, "Failed to init vault config")
	}
	client, err := vaultapi.NewClient(config)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create vault client")
	}
	return &HTTPVaultClient{
		client:  client,
		sys:     client.Sys(),
		logical: client.Logical(),
	}, nil
}

// PutPolicy uploads a policy to vault
func (v *HTTPVaultClient) PutPolicy(name, rules string) error {
	if err := v.sys.PutPolicy(name, rules); err != nil {
		return kerrors.WithMsg(err, "Failed to store vault policy")
	}
	return nil
}

// PutRole uploads a role to vault
func (v *HTTPVaultClient) PutRole(mount, name string, body map[string]interface{}) error {
	if _, err := v.logical.Write(fmt.Sprintf("auth/%s/role/%s", mount, name), body); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to store vault role %s for auth %s", name, mount))
	}
	return nil
}

type (
	// Opts are vault client opts
	Opts struct {
		DryRun bool
	}
)

// AddPolicyDir uploads policies from a directory to vault
func AddPolicyDir(ctx context.Context, log klog.Logger, client VaultClient, fsys fs.FS, opts Opts) error {
	l := klog.NewLevelLogger(log)

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to read dir")
	}
	for _, i := range entries {
		if i.IsDir() {
			continue
		}
		name := i.Name()
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed to read file: %s", name))
		}
		policyName := strings.TrimSuffix(name, path.Ext(name))
		if opts.DryRun {
			l.Info(ctx, "Dry run upload vault policy", klog.AString("policy", policyName))
		} else {
			if err := client.PutPolicy(policyName, string(b)); err != nil {
				return kerrors.WithMsg(err, fmt.Sprintf("Failed to upload vault policy: %s", policyName))
			}
			l.Info(ctx, "Uploaded vault policy", klog.AString("policy", policyName))
		}
		return nil
	}
	return nil
}

// AddPolicies creates a vault client and uploads policies from a directory
func AddPolicies(ctx context.Context, log klog.Logger, name string, opts Opts) error {
	client, err := NewHTTPVaultClient()
	if err != nil {
		return err
	}
	if err := AddPolicyDir(ctx, log, client, os.DirFS(filepath.FromSlash(name)), opts); err != nil {
		return err
	}
	return nil
}

type (
	// roleData is the shape of a role
	roleData struct {
		Kind      string   `json:"kind"`
		KubeMount string   `json:"kubemount"`
		Role      string   `json:"role"`
		SA        string   `json:"service_account,omitempty"`
		NS        string   `json:"namespace,omitempty"`
		Policies  []string `json:"policies"`
		TTL       string   `json:"ttl"`
		MaxTTL    string   `json:"maxttl"`
	}

	// roleConfigData is the shape of a role config
	roleConfigData struct {
		Roles []roleData `json:"roles"`
	}
)

// AddRolesDir uploads roles from a directory to vault
func AddRolesDir(ctx context.Context, log klog.Logger, client VaultClient, fsys fs.FS, opts Opts) error {
	l := klog.NewLevelLogger(log)

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to read dir")
	}

	var roles []roleData
	for _, i := range entries {
		if i.IsDir() {
			continue
		}
		name := i.Name()
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed to read file: %s", name))
		}
		var roleConfig roleConfigData
		if err := kjson.Unmarshal(b, &roleConfig); err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Invalid vault roles file: %s", name))
		}
		roles = append(roles, roleConfig.Roles...)
	}

	for _, i := range roles {
		switch i.Kind {
		case roleKindKube:
			body := map[string]interface{}{
				"bound_service_account_names":      i.SA,
				"bound_service_account_namespaces": i.NS,
				"token_policies":                   i.Policies,
				"token_ttl":                        i.TTL,
				"token_max_ttl":                    i.MaxTTL,
				"token_no_default_policy":          true,
			}
			if opts.DryRun {
				l.Info(ctx, "Dry run upload vault role", klog.AString("role", i.Role))
			} else {
				if err := client.PutRole(i.KubeMount, i.Role, body); err != nil {
					return kerrors.WithMsg(err, fmt.Sprintf("Failed to upload vault role: %s", i.Role))
				}
				l.Info(ctx, "Uploaded vault role", klog.AString("role", i.Role))
			}
		default:
			return kerrors.WithKind(nil, ErrInvalidRoleKind, fmt.Sprintf("Invalid role kind: %s", i.Kind))
		}
	}
	return nil
}

// AddRoles creates a vault client and uploads roles from a directory
func AddRoles(ctx context.Context, log klog.Logger, name string, opts Opts) error {
	client, err := NewHTTPVaultClient()
	if err != nil {
		return err
	}
	if err := AddRolesDir(ctx, log, client, os.DirFS(filepath.FromSlash(name)), opts); err != nil {
		return err
	}
	return nil
}
