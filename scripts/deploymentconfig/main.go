package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"
	"golang.org/x/xerrors"
)

const (
	StructTarget = "DeploymentConfig"
	EnvPrefix    = "CODER_"
)

func main() {
	data, err := GenerateData(context.Background(), "./codersdk")
	if err != nil {
		panic(err.Error())
	}

	// Just cat the output to a file to capture it
	_, _ = fmt.Println(Render(data))
}

type Data struct {
	StructTarget string
	Fields       []Field
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
	ViperType  string
}

func GenerateData(ctx context.Context, dir string) (*Data, error) {
	pkg, err := parsePackage(ctx, dir)
	if err != nil {
		return nil, xerrors.Errorf("parse package %q: %w", dir, err)
	}

	codeBlocks, err := generateAll(pkg)
	if err != nil {
		return nil, xerrors.Errorf("parse package %q: %w", dir, err)
	}

	return codeBlocks, nil
}

// parsePackage takes a list of patterns such as a directory, and parses them.
func parsePackage(ctx context.Context, patterns ...string) (*packages.Package, error) {
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
		return nil, xerrors.Errorf("load package: %w", err)
	}

	// Only support 1 package for now. We can expand it if we need later, we
	// just need to hook up multiple packages in the generator.
	if len(pkgs) != 1 {
		return nil, xerrors.Errorf("expected 1 package, found %d", len(pkgs))
	}

	return pkgs[0], nil
}

func generateAll(pkg *packages.Package) (*Data, error) {
	cb := Data{}
	structs := make(map[string]*ast.StructType)
	for _, file := range pkg.Syntax {
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
				t, ok := spec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				structs[spec.Name.Name] = t
			}
		}
	}

	cb.Fields = handleStruct("", StructTarget, structs, cb.Fields)

	return &cb, nil
}

func handleStruct(prefix string, target string, structs map[string]*ast.StructType, fields []Field) []Field {
	var dc *ast.StructType
	for name, t := range structs {
		if name == target {
			dc = t
			break
		}
	}
	if dc == nil {
		return fields
	}
	for _, field := range dc.Fields.List {
		key := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("mapstructure")
		if key == "" {
			continue
		}
		if prefix != "" {
			key = fmt.Sprintf("%s.%s", prefix, key)
		}
		f := Field{
			Key: key,
			Env: EnvPrefix + strings.ReplaceAll(strings.ToUpper(key), "-", "_"),
		}
		switch ft := field.Type.(type) {
		case *ast.Ident:
			switch ft.Name {
			case "string":
				f.Type = "String"
				f.ViperType = "String"
			case "int":
				f.Type = "Int"
				f.ViperType = "Int"
			case "bool":
				f.Type = "Bool"
				f.ViperType = "Bool"
			default:
				_, ok := structs[ft.Name]
				if !ok {
					continue
				}
				fields = handleStruct(key, ft.Name, structs, fields)
				continue
			}
		case *ast.SelectorExpr:
			if ft.Sel.Name != "Duration" {
				continue
			}
			f.Type = "Duration"
			f.ViperType = "Duration"
		case *ast.ArrayType:
			i, ok := ft.Elt.(*ast.Ident)
			if !ok {
				continue
			}
			if i.Name != "string" {
				continue
			}
			f.Type = "StringArray"
			f.ViperType = "StringSlice"
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
					continue
				}
				f.Enterprise = b
			}
			if strings.HasPrefix(line.Text, "// Hidden:") {
				v := strings.TrimPrefix(line.Text, "// Hidden:")
				v = strings.TrimSpace(v)
				v = strings.TrimSpace(v)
				b, err := strconv.ParseBool(v)
				if err != nil {
					continue
				}
				f.Hidden = b
			}
		}

		fields = append(fields, f)
	}
	return fields
}

func Render(d *Data) string {
	t, err := template.New(StructTarget).Parse(deploymentConfigTemplate)
	if err != nil {
		panic(err)
	}
	var b bytes.Buffer
	err = t.Execute(&b, d)
	if err != nil {
		panic(err)
	}

	return b.String()
}

const deploymentConfigTemplate = `// Code generated by go generate; DO NOT EDIT.
// This file was generated by the script at scripts/deploymentconfig/main.go
// The data for populating this file is from codersdk.{{ .StructTarget }}.
package deployment

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
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
	_ = flagset.{{ .Type }}P("{{ .Flag }}", "{{ .Shorthand }}", vip.Get{{ .ViperType }}("{{ .Key }}"), ` + "`{{ .Usage }}`" + `+"\n"+cliui.Styles.Placeholder.Render("Consumes ${{ .Env }}"))
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
	_ = flagset.{{ .Type }}P("{{ .Flag }}", "{{ .Shorthand }}", vip.Get{{ .ViperType }}("{{ .Key }}"), ` + "`{{ .Usage }}`" + `)
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
