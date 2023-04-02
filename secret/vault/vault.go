package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

const (
	roleKindKube = "kube"
)

// ErrInvalidRoleKind is returned when attempting to parse a role with an invalid kind
var ErrInvalidRoleKind = errors.New("Invalid role kind")

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
		return nil, fmt.Errorf("Failed to init vault config: %w", err)
	}
	client, err := vaultapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to create vault client: %w", err)
	}
	return &HTTPVaultClient{
		client: client,
	}, nil
}

// PutPolicy uploads a policy to vault
func (v *HTTPVaultClient) PutPolicy(name, rules string) error {
	if v.sys == nil {
		v.sys = v.client.Sys()
	}
	return v.sys.PutPolicy(name, rules)
}

// PutRole uploads a role to vault
func (v *HTTPVaultClient) PutRole(mount, name string, body map[string]interface{}) error {
	if v.logical == nil {
		v.logical = v.client.Logical()
	}
	_, err := v.logical.Write(fmt.Sprintf("auth/%s/role/%s", mount, name), body)
	return err
}

type (
	// Opts are vault client opts
	Opts struct {
		Verbose bool
		DryRun  bool
	}
)

// AddPolicyDir uploads policies from a directory to vault
func AddPolicyDir(ctx context.Context, client VaultClient, dir fs.FS, opts Opts) error {
	entries, err := fs.ReadDir(dir, ".")
	if err != nil {
		return fmt.Errorf("Failed to read dir: %w", err)
	}
	for _, i := range entries {
		if i.IsDir() {
			continue
		}
		if err := func() error {
			name := i.Name()
			file, err := dir.Open(name)
			if err != nil {
				return fmt.Errorf("Invalid file %s: %w", name, err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					log.Printf("Failed to close open file %s: %v", name, err)
				}
			}()
			b := &bytes.Buffer{}
			if _, err := io.Copy(b, file); err != nil {
				return fmt.Errorf("Failed to read file %s: %w", name, err)
			}
			base := filepath.Base(name)
			policyName := strings.TrimSuffix(base, filepath.Ext(base))
			if opts.Verbose {
				log.Printf("Uploading policy %s", policyName)
			}
			if !opts.DryRun {
				if err := client.PutPolicy(policyName, b.String()); err != nil {
					return fmt.Errorf("Failed to upload policy %s to vault: %w", policyName, err)
				}
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

// AddPolicies creates a vault client and uploads policies from a directory
func AddPolicies(ctx context.Context, path string, opts Opts) error {
	client, err := NewHTTPVaultClient()
	if err != nil {
		return err
	}
	dir := os.DirFS(path)
	if err := AddPolicyDir(ctx, client, dir, opts); err != nil {
		return err
	}
	return nil
}

type (
	// roleData is the shape of a role
	roleData struct {
		Kind      string   `json:"kind" yaml:"kind"`
		KubeMount string   `json:"kubemount" yaml:"kubemount"`
		Role      string   `json:"role" yaml:"role"`
		SA        string   `json:"service_account,omitempty" yaml:"service_account,omitempty"`
		NS        string   `json:"namespace,omitempty" yaml:"namespace,omitempty"`
		Policies  []string `json:"policies" yaml:"policies"`
		TTL       string   `json:"ttl" yaml:"ttl"`
		MaxTTL    string   `json:"maxttl" yaml:"maxttl"`
	}

	// roleConfigData is the shape of a role config
	roleConfigData struct {
		Roles []roleData `json:"roles"`
	}
)

// AddRolesDir uploads roles from a directory to vault
func AddRolesDir(ctx context.Context, client VaultClient, dir fs.FS, opts Opts) error {
	entries, err := fs.ReadDir(dir, ".")
	if err != nil {
		return fmt.Errorf("Failed to read dir: %w", err)
	}

	var roles []roleData
	for _, i := range entries {
		if i.IsDir() {
			continue
		}
		if err := func() error {
			name := i.Name()
			var roleConfig roleConfigData
			if err := configfile.DecodeJSONorYAMLFile(dir, name, &roleConfig); err != nil {
				return fmt.Errorf("Invalid vault roles file %s: %w", name, err)
			}
			roles = append(roles, roleConfig.Roles...)
			return nil
		}(); err != nil {
			return err
		}
	}

	for _, i := range roles {
		switch i.Kind {
		case roleKindKube:
			body := map[string]interface{}{
				"bound_service_account_names":      i.SA,
				"bound_service_account_namespaces": i.NS,
				"policies":                         i.Policies,
				"ttl":                              i.TTL,
				"max_ttl":                          i.MaxTTL,
			}
			if opts.Verbose {
				log.Printf("Uploading role %s: %v", i.Role, body)
			}
			if !opts.DryRun {
				if err := client.PutRole(i.KubeMount, i.Role, body); err != nil {
					return fmt.Errorf("Failed to upload role %s to vault: %w", i.Role, err)
				}
			}
		default:
			return fmt.Errorf("%w: %s", ErrInvalidRoleKind, i.Kind)
		}
	}
	return nil
}

// AddRoles creates a vault client and uploads roles from a directory
func AddRoles(ctx context.Context, path string, opts Opts) error {
	client, err := NewHTTPVaultClient()
	if err != nil {
		return err
	}
	dir := os.DirFS(path)
	if err := AddRolesDir(ctx, client, dir, opts); err != nil {
		return err
	}
	return nil
}
