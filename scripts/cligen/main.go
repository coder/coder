package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
)

const (
	PkgDir = "./codersdk"
)

func main() {
	ctx := context.Background()
	log := slog.Make(sloghuman.Sink(os.Stderr))
	data, err := GenerateData(ctx, log, PkgDir)
	if err != nil {
		log.Fatal(ctx, err.Error())
	}

	// Just cat the output to a file to capture it
	_, _ = fmt.Println(data.Render())
}

type Data struct {
	Fields []Field
}

type Field struct {
	Key        string
	Env        string
	Usage      string
	Flag       string
	Shorthand  string
	Default    string
	Enterprise bool
	Hidden     bool
	Type       string
}

func GenerateData(ctx context.Context, log slog.Logger, dir string) (*Data, error) {
	g := Generator{
		log: log,
	}
	err := g.parsePackage(ctx, dir)
	if err != nil {
		return nil, xerrors.Errorf("parse package %q: %w", dir, err)
	}

	codeBlocks, err := g.generateAll()
	if err != nil {
		return nil, xerrors.Errorf("parse package %q: %w", dir, err)
	}

	return codeBlocks, nil
}

type Generator struct {
	// Package we are scanning.
	pkg *packages.Package
	log slog.Logger
}

// parsePackage takes a list of patterns such as a directory, and parses them.
func (g *Generator) parsePackage(ctx context.Context, patterns ...string) error {
	cfg := &packages.Config{
		// Just accept the fact we need these flags for what we want. Feel free to add
		// more, it'll just increase the time it takes to parse.
		Mode: packages.NeedTypes | packages.NeedName | packages.NeedTypesInfo |
			packages.NeedTypesSizes | packages.NeedSyntax,
		Tests:   false,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return xerrors.Errorf("load package: %w", err)
	}

	// Only support 1 package for now. We can expand it if we need later, we
	// just need to hook up multiple packages in the generator.
	if len(pkgs) != 1 {
		return xerrors.Errorf("expected 1 package, found %d", len(pkgs))
	}

	g.pkg = pkgs[0]
	return nil
}

func (g *Generator) generateAll() (*Data, error) {
	cb := Data{}
	for _, file := range g.pkg.Syntax {
		for _, decl := range file.Decls {
			decl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			if decl.Tok != token.TYPE {
				continue
			}
			for _, speci := range decl.Specs {
				spec, ok := speci.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if spec.Name.Name != "DeploymentConfig" {
					continue
				}
				t, ok := spec.Type.(*ast.StructType)
				if !ok {
					return nil, xerrors.Errorf("expected struct type, found %T", spec.Type)
				}
				for _, field := range t.Fields.List {
					key := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("mapstructure")
					if key == "" {
						continue
					}
					f := Field{
						Key: key,
						Env: "CODER_" + strings.ReplaceAll(strings.ToUpper(key), "-", "_"),
					}
					ft, ok := field.Type.(*ast.Ident)
					if !ok {
						// return nil, xerrors.Errorf("expected Ident type, found %T", field.Type)
						continue
					}
					switch ft.Name {
					case "string":
						f.Type = "String"
					case "int":
						f.Type = "Int"
					case "bool":
						f.Type = "Bool"
					case "[]string":
						f.Type = "StringArray"
					case "time.Duration":
						f.Type = "Duration"
					default:
						continue
					}

					for _, line := range field.Doc.List {
						if strings.HasPrefix(line.Text, "// Usage:") {
							v := strings.TrimPrefix(line.Text, "// Usage:")
							v = strings.TrimSpace(v)
							f.Usage = v
						}
						if strings.HasPrefix(line.Text, "// Flag:") {
							v := strings.TrimPrefix(line.Text, "// Flag:")
							v = strings.TrimSpace(v)
							f.Flag = v
						}
						if strings.HasPrefix(line.Text, "// Shorthand:") {
							v := strings.TrimPrefix(line.Text, "// Shorthand:")
							v = strings.TrimSpace(v)
							f.Shorthand = v
						}
						if strings.HasPrefix(line.Text, "// Default:") {
							v := strings.TrimPrefix(line.Text, "// Default:")
							v = strings.TrimSpace(v)
							f.Default = v
						}
						if strings.HasPrefix(line.Text, "// Enterprise:") {
							v := strings.TrimPrefix(line.Text, "// Enterprise:")
							v = strings.TrimSpace(v)
							b, err := strconv.ParseBool(v)
							if err != nil {
								return nil, xerrors.Errorf("parse enterprise: %w", err)
							}
							f.Enterprise = b
						}
						if strings.HasPrefix(line.Text, "// Hidden:") {
							v := strings.TrimPrefix(line.Text, "// Hidden:")
							v = strings.TrimSpace(v)
							v = strings.TrimSpace(v)
							b, err := strconv.ParseBool(v)
							if err != nil {
								return nil, xerrors.Errorf("parse hidden: %w", err)
							}
							f.Hidden = b
						}
					}

					cb.Fields = append(cb.Fields, f)
				}
			}
		}
	}

	return &cb, nil
}

func (c Data) Render() string {
	t, err := template.New("DeploymentConfig").Parse(deploymentConfigTemplate)
	if err != nil {
		panic(err)
	}
	var b bytes.Buffer
	err = t.Execute(&b, c)
	if err != nil {
		panic(err)
	}

	return b.String()
}

const deploymentConfigTemplate = `// Code generated by go generate; DO NOT EDIT.
// This file was generated by the script at scripts/cligen
// The data for populating this file is from the DeploymentConfig struct in codersdk.
package deployment

import (
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cli/cliui"
)

func Config(vip *viper.Viper) (codersdk.DeploymentConfig, error) {
	cfg := codersdk.DeploymentConfig{}
	return cfg, vip.Unmarshal(cfg)
}

func DefaultViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("coder")
	v.AutomaticEnv()
	{{- range .Fields }}
	{{- if .Default }}
	v.SetDefault("{{ .Key }}", {{ .Default }})
	{{- end }}
	{{- end }}

	return v
}

func AttachFlags(flagset *pflag.FlagSet, vip *viper.Viper) {
	{{- range .Fields }}
	{{- if and (.Flag) (not .Enterprise) }}
	_ = flagset.{{ .Type }}P("{{ .Flag }}", "{{ .Shorthand }}", vip.Get{{ .Type }}("{{ .Key }}"), ` + "`{{ .Usage }}`" + `+"\n"+cliui.Styles.Placeholder.Render("Consumes ${{ .Env }}"))
	_ = vip.BindPFlag("{{ .Key }}", flagset.Lookup("{{ .Flag }}"))
	{{- if and .Hidden }}
	_ = flagset.MarkHidden("{{ .Flag }}")
	{{- end }}
	{{- end }}
	{{- end }}
}

func AttachEnterpriseFlags(flagset *pflag.FlagSet, vip *viper.Viper) {
	{{- range .Fields }}
	{{- if and (.Flag) (.Enterprise) }}
	_ = flagset.{{ .Type }}P("{{ .Flag }}", "{{ .Shorthand }}", vip.Get{{ .Type }}("{{ .Key }}"), ` + "`{{ .Usage }}`" + `)
	_ = vip.BindPFlag("{{ .Key }}", flagset.Lookup("{{ .Flag }}"))
	{{- if and .Hidden }}
	_ = flagset.MarkHidden("{{ .Flag }}")
	{{- end }}
	{{- end }}
	{{- end }}
}

func defaultCacheDir() string {
	defaultCacheDir, err := os.UserCacheDir()
	if err != nil {
		defaultCacheDir = os.TempDir()
	}
	if dir := os.Getenv("CACHE_DIRECTORY"); dir != "" {
		// For compatibility with systemd.
		defaultCacheDir = dir
	}

	return filepath.Join(defaultCacheDir, "coder")
}
`
