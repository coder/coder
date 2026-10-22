package networkpolicy

import (
	"archive/tar"
	"bytes"
	"io/fs"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"golang.org/x/xerrors"
)

const maxArchiveSize = 4 << 20

// Archive freezes a policy's HCL and trusted module into a Terraform package.
// Only root module .tf files and the provider lockfile are copied. Credentials,
// state, downloaded providers, tests, and nested modules are never packaged.
func Archive(source string, module fs.FS) ([]byte, error) {
	if module == nil {
		return nil, xerrors.New("network policy module is not configured")
	}
	if err := ValidateHCL(source); err != nil {
		return nil, err
	}
	file, diags := hclwrite.ParseConfig([]byte(source), "main.tf", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, xerrors.Errorf("parse policy package: %s", diags.Error())
	}
	body := file.Body().Blocks()[0].Body()
	for _, name := range []string{"policy_id", "namespace", "template_ids"} {
		body.SetAttributeTraversal(name, hcl.Traversal{
			hcl.TraverseRoot{Name: "var"}, hcl.TraverseAttr{Name: name},
		})
	}

	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	// The provisioner's extractor requires parent directories before files.
	// Emit each ancestor once, without copying unapproved module directories.
	directories := map[string]bool{}
	var addDirectory func(string) error
	addDirectory = func(name string) error {
		if name == "." || directories[name] {
			return nil
		}
		if err := addDirectory(path.Dir(name)); err != nil {
			return err
		}
		if err := writer.WriteHeader(&tar.Header{Name: name + "/", Mode: 0o700, Typeflag: tar.TypeDir}); err != nil {
			return xerrors.Errorf("write policy archive directory: %w", err)
		}
		directories[name] = true
		return nil
	}
	add := func(name string, data []byte) error {
		if err := addDirectory(path.Dir(name)); err != nil {
			return err
		}
		if buffer.Len()+len(data) > maxArchiveSize {
			return xerrors.Errorf("policy package exceeds %d bytes", maxArchiveSize)
		}
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return xerrors.Errorf("write policy archive header: %w", err)
		}
		if _, err := writer.Write(data); err != nil {
			return xerrors.Errorf("write policy archive content: %w", err)
		}
		return nil
	}
	if err := add("main.tf", file.Bytes()); err != nil {
		return nil, err
	}
	if err := add("coder.tf", []byte(rootConfiguration)); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(module, ".")
	if err != nil {
		return nil, xerrors.Errorf("read network policy module: %w", err)
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".tf") && name != ".terraform.lock.hcl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, xerrors.Errorf("stat module file %q: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Size() > maxArchiveSize {
			return nil, xerrors.Errorf("module file %q must be a regular file smaller than %d bytes", name, maxArchiveSize)
		}
		data, err := fs.ReadFile(module, name)
		if err != nil {
			return nil, xerrors.Errorf("read module file %q: %w", name, err)
		}
		archivePath := "module/" + name
		if name == ".terraform.lock.hcl" {
			archivePath = name
		} else {
			count++
		}
		if err := add(archivePath, data); err != nil {
			return nil, err
		}
	}
	if count == 0 {
		return nil, xerrors.New("network policy module contains no Terraform files")
	}
	if err := writer.Close(); err != nil {
		return nil, xerrors.Errorf("close policy archive: %w", err)
	}
	return buffer.Bytes(), nil
}

const rootConfiguration = `terraform {
  required_version = ">= 1.9.0, < 2.0.0"
  required_providers {
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = "= 2.38.0"
    }
  }
}

variable "policy_id" {
  type = string
}
variable "namespace" {
  type = string
}
variable "template_ids" {
  type = list(string)
}
variable "kubeconfig_context" {
  type = string
}

provider "kubernetes" {
  config_context = var.kubeconfig_context
}
`
