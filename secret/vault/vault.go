package vault

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

type (
	VaultClient interface {
		PutPolicy(name, rules string) error
	}

	HTTPVaultClient struct {
		client *vaultapi.Client
	}
)

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

func (v *HTTPVaultClient) PutPolicy(name, rules string) error {
	return v.client.Sys().PutPolicy(name, rules)
}

// AddPolicyDir uploads policies from a directory to vault
func AddPolicyDir(ctx context.Context, client VaultClient, dir fs.FS) error {
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
			if err := client.PutPolicy(policyName, b.String()); err != nil {
				return fmt.Errorf("Failed to upload policy %s to vault: %w", policyName, err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

// AddPolicies creates a vault client and uploads policies from a directory
func AddPolicies(ctx context.Context, path string) error {
	client, err := NewHTTPVaultClient()
	if err != nil {
		return err
	}
	dir := os.DirFS(path)
	if err := AddPolicyDir(ctx, client, dir); err != nil {
		return err
	}
	return nil
}
