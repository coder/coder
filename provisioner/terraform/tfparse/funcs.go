package tfparse
import (
	"fmt"
	"errors"
	"github.com/aquasecurity/trivy-iac/pkg/scanners/terraform/parser/funcs"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)
// Functions returns a set of functions that are safe to use in the context of
// evaluating Terraform expressions without any ability to reference local files.
// Functions that refer to file operations are replaced with stubs that return a
// descriptive error to the user.
func Functions() map[string]function.Function {
	return allFunctions
}
var (
	// Adapted from github.com/aquasecurity/trivy-iac@v0.8.0/pkg/scanners/terraform/parser/functions.go
	// We cannot support all available functions here, as the result of reading a file will be different
	// depending on the execution environment.
	safeFunctions = map[string]function.Function{
		"abs":             stdlib.AbsoluteFunc,
		"basename":        funcs.BasenameFunc,
		"base64decode":    funcs.Base64DecodeFunc,
		"base64encode":    funcs.Base64EncodeFunc,
		"base64gzip":      funcs.Base64GzipFunc,
		"base64sha256":    funcs.Base64Sha256Func,
		"base64sha512":    funcs.Base64Sha512Func,
		"bcrypt":          funcs.BcryptFunc,
		"can":             tryfunc.CanFunc,
		"ceil":            stdlib.CeilFunc,
		"chomp":           stdlib.ChompFunc,
		"cidrhost":        funcs.CidrHostFunc,
		"cidrnetmask":     funcs.CidrNetmaskFunc,
		"cidrsubnet":      funcs.CidrSubnetFunc,
		"cidrsubnets":     funcs.CidrSubnetsFunc,
		"coalesce":        funcs.CoalesceFunc,
		"coalescelist":    stdlib.CoalesceListFunc,
		"compact":         stdlib.CompactFunc,
		"concat":          stdlib.ConcatFunc,
		"contains":        stdlib.ContainsFunc,
		"csvdecode":       stdlib.CSVDecodeFunc,
		"dirname":         funcs.DirnameFunc,
		"distinct":        stdlib.DistinctFunc,
		"element":         stdlib.ElementFunc,
		"chunklist":       stdlib.ChunklistFunc,
		"flatten":         stdlib.FlattenFunc,
		"floor":           stdlib.FloorFunc,
		"format":          stdlib.FormatFunc,
		"formatdate":      stdlib.FormatDateFunc,
		"formatlist":      stdlib.FormatListFunc,
		"indent":          stdlib.IndentFunc,
		"index":           funcs.IndexFunc, // stdlib.IndexFunc is not compatible
		"join":            stdlib.JoinFunc,
		"jsondecode":      stdlib.JSONDecodeFunc,
		"jsonencode":      stdlib.JSONEncodeFunc,
		"keys":            stdlib.KeysFunc,
		"length":          funcs.LengthFunc,
		"list":            funcs.ListFunc,
		"log":             stdlib.LogFunc,
		"lookup":          funcs.LookupFunc,
		"lower":           stdlib.LowerFunc,
		"map":             funcs.MapFunc,
		"matchkeys":       funcs.MatchkeysFunc,
		"max":             stdlib.MaxFunc,
		"md5":             funcs.Md5Func,
		"merge":           stdlib.MergeFunc,
		"min":             stdlib.MinFunc,
		"parseint":        stdlib.ParseIntFunc,
		"pow":             stdlib.PowFunc,
		"range":           stdlib.RangeFunc,
		"regex":           stdlib.RegexFunc,
		"regexall":        stdlib.RegexAllFunc,
		"replace":         funcs.ReplaceFunc,
		"reverse":         stdlib.ReverseListFunc,
		"rsadecrypt":      funcs.RsaDecryptFunc,
		"setintersection": stdlib.SetIntersectionFunc,
		"setproduct":      stdlib.SetProductFunc,
		"setsubtract":     stdlib.SetSubtractFunc,
		"setunion":        stdlib.SetUnionFunc,
		"sha1":            funcs.Sha1Func,
		"sha256":          funcs.Sha256Func,
		"sha512":          funcs.Sha512Func,
		"signum":          stdlib.SignumFunc,
		"slice":           stdlib.SliceFunc,
		"sort":            stdlib.SortFunc,
		"split":           stdlib.SplitFunc,
		"strrev":          stdlib.ReverseFunc,
		"substr":          stdlib.SubstrFunc,
		"timestamp":       funcs.TimestampFunc,
		"timeadd":         stdlib.TimeAddFunc,
		"title":           stdlib.TitleFunc,
		"tostring":        funcs.MakeToFunc(cty.String),
		"tonumber":        funcs.MakeToFunc(cty.Number),
		"tobool":          funcs.MakeToFunc(cty.Bool),
		"toset":           funcs.MakeToFunc(cty.Set(cty.DynamicPseudoType)),
		"tolist":          funcs.MakeToFunc(cty.List(cty.DynamicPseudoType)),
		"tomap":           funcs.MakeToFunc(cty.Map(cty.DynamicPseudoType)),
		"transpose":       funcs.TransposeFunc,
		"trim":            stdlib.TrimFunc,
		"trimprefix":      stdlib.TrimPrefixFunc,
		"trimspace":       stdlib.TrimSpaceFunc,
		"trimsuffix":      stdlib.TrimSuffixFunc,
		"try":             tryfunc.TryFunc,
		"upper":           stdlib.UpperFunc,
		"urlencode":       funcs.URLEncodeFunc,
		"uuid":            funcs.UUIDFunc,
		"uuidv5":          funcs.UUIDV5Func,
		"values":          stdlib.ValuesFunc,
		"yamldecode":      ctyyaml.YAMLDecodeFunc,
		"yamlencode":      ctyyaml.YAMLEncodeFunc,
		"zipmap":          stdlib.ZipmapFunc,
	}
	// the below functions are not safe for usage in the context of tfparse, as their return
	// values may change depending on the underlying filesystem.
	stubFileFunctions = map[string]function.Function{
		"abspath":          makeStubFunction("abspath", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"file":             makeStubFunction("file", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"fileexists":       makeStubFunction("fileexists", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"fileset":          makeStubFunction("fileset", cty.String, function.Parameter{Name: "path", Type: cty.String}, function.Parameter{Name: "pattern", Type: cty.String}),
		"filebase64":       makeStubFunction("filebase64", cty.String, function.Parameter{Name: "path", Type: cty.String}, function.Parameter{Name: "pattern", Type: cty.String}),
		"filebase64sha256": makeStubFunction("filebase64sha256", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"filebase64sha512": makeStubFunction("filebase64sha512", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"filemd5":          makeStubFunction("filemd5", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"filesha1":         makeStubFunction("filesha1", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"filesha256":       makeStubFunction("filesha256", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"filesha512":       makeStubFunction("filesha512", cty.String, function.Parameter{Name: "path", Type: cty.String}),
		"pathexpand":       makeStubFunction("pathexpand", cty.String, function.Parameter{Name: "path", Type: cty.String}),
	}
	allFunctions = mergeMaps(safeFunctions, stubFileFunctions)
)
// mergeMaps returns a new map which is the result of merging each key and value
// of all maps in ms, in order. Successive maps may override values of previous
// maps.
func mergeMaps[K, V comparable](ms ...map[K]V) map[K]V {
	merged := make(map[K]V)
	for _, m := range ms {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}
// makeStubFunction returns a function.Function with the required return type and parameters
// that will always return an unknown type and an error.
func makeStubFunction(name string, returnType cty.Type, params ...function.Parameter) function.Function {
	var spec function.Spec
	spec.Params = params
	spec.Type = function.StaticReturnType(returnType)
	spec.Impl = func(_ []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.UnknownVal(returnType), fmt.Errorf("function %q may not be used here", name)
	}
	return function.New(&spec)
}
