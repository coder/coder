package clibase

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// NoOptDefValuer describes behavior when no
// option is passed into the flag.
//
// This is useful for boolean or otherwise binary flags.
type NoOptDefValuer interface {
	NoOptDefValue() string
}

// Validator is a wrapper around a pflag.Value that allows for validation
// of the value after or before it has been set.
type Validator[T pflag.Value] struct {
	Value T
	// validate is called after the value is set.
	validate func(T) error
}

func Validate[T pflag.Value](opt T, validate func(value T) error) *Validator[T] {
	return &Validator[T]{Value: opt, validate: validate}
}

func (i *Validator[T]) String() string {
	return i.Value.String()
}

func (i *Validator[T]) Set(input string) error {
	err := i.Value.Set(input)
	if err != nil {
		return err
	}
	if i.validate != nil {
		err = i.validate(i.Value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *Validator[T]) Type() string {
	return i.Value.Type()
}

// values.go contains a standard set of value types that can be used as
// Option Values.

type Int64 int64

func Int64Of(i *int64) *Int64 {
	return (*Int64)(i)
}

func (i *Int64) Set(s string) error {
	ii, err := strconv.ParseInt(s, 10, 64)
	*i = Int64(ii)
	return err
}

func (i Int64) Value() int64 {
	return int64(i)
}

func (i Int64) String() string {
	return strconv.Itoa(int(i))
}

func (Int64) Type() string {
	return "int"
}

type Bool bool

func BoolOf(b *bool) *Bool {
	return (*Bool)(b)
}

func (b *Bool) Set(s string) error {
	if s == "" {
		*b = Bool(false)
		return nil
	}
	bb, err := strconv.ParseBool(s)
	*b = Bool(bb)
	return err
}

func (*Bool) NoOptDefValue() string {
	return "true"
}

func (b Bool) String() string {
	return strconv.FormatBool(bool(b))
}

func (b Bool) Value() bool {
	return bool(b)
}

func (Bool) Type() string {
	return "bool"
}

type String string

func StringOf(s *string) *String {
	return (*String)(s)
}

func (*String) NoOptDefValue() string {
	return ""
}

func (s *String) Set(v string) error {
	*s = String(v)
	return nil
}

func (s String) String() string {
	return string(s)
}

func (s String) Value() string {
	return string(s)
}

func (String) Type() string {
	return "string"
}

var _ pflag.SliceValue = &StringArray{}

// StringArray is a slice of strings that implements pflag.Value and pflag.SliceValue.
type StringArray []string

func StringArrayOf(ss *[]string) *StringArray {
	return (*StringArray)(ss)
}

func (s *StringArray) Append(v string) error {
	*s = append(*s, v)
	return nil
}

func (s *StringArray) Replace(vals []string) error {
	*s = vals
	return nil
}

func (s *StringArray) GetSlice() []string {
	return *s
}

func readAsCSV(v string) ([]string, error) {
	return csv.NewReader(strings.NewReader(v)).Read()
}

func writeAsCSV(vals []string) string {
	var sb strings.Builder
	err := csv.NewWriter(&sb).Write(vals)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	return sb.String()
}

func (s *StringArray) Set(v string) error {
	if v == "" {
		*s = nil
		return nil
	}
	ss, err := readAsCSV(v)
	if err != nil {
		return err
	}
	*s = append(*s, ss...)
	return nil
}

func (s StringArray) String() string {
	return writeAsCSV([]string(s))
}

func (s StringArray) Value() []string {
	return []string(s)
}

func (StringArray) Type() string {
	return "string-array"
}

type Duration time.Duration

func DurationOf(d *time.Duration) *Duration {
	return (*Duration)(d)
}

func (d *Duration) Set(v string) error {
	dd, err := time.ParseDuration(v)
	*d = Duration(dd)
	return err
}

func (d *Duration) Value() time.Duration {
	return time.Duration(*d)
}

func (d *Duration) String() string {
	return time.Duration(*d).String()
}

func (Duration) Type() string {
	return "duration"
}

func (d *Duration) MarshalYAML() (interface{}, error) {
	return yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: d.String(),
	}, nil
}

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	return d.Set(n.Value)
}

type URL url.URL

func URLOf(u *url.URL) *URL {
	return (*URL)(u)
}

func (u *URL) Set(v string) error {
	uu, err := url.Parse(v)
	if err != nil {
		return err
	}
	*u = URL(*uu)
	return nil
}

func (u *URL) String() string {
	uu := url.URL(*u)
	return uu.String()
}

func (u *URL) MarshalYAML() (interface{}, error) {
	return yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: u.String(),
	}, nil
}

func (u *URL) UnmarshalYAML(n *yaml.Node) error {
	return u.Set(n.Value)
}

func (u *URL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

func (u *URL) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	return u.Set(s)
}

func (*URL) Type() string {
	return "url"
}

func (u *URL) Value() *url.URL {
	return (*url.URL)(u)
}

// HostPort is a host:port pair.
type HostPort struct {
	Host string
	Port string
}

func (hp *HostPort) Set(v string) error {
	if v == "" {
		return xerrors.Errorf("must not be empty")
	}
	var err error
	hp.Host, hp.Port, err = net.SplitHostPort(v)
	return err
}

func (hp *HostPort) String() string {
	if hp.Host == "" && hp.Port == "" {
		return ""
	}
	// Warning: net.JoinHostPort must be used over concatenation to support
	// IPv6 addresses.
	return net.JoinHostPort(hp.Host, hp.Port)
}

func (hp *HostPort) MarshalJSON() ([]byte, error) {
	return json.Marshal(hp.String())
}

func (hp *HostPort) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	if s == "" {
		hp.Host = ""
		hp.Port = ""
		return nil
	}
	return hp.Set(s)
}

func (hp *HostPort) MarshalYAML() (interface{}, error) {
	return yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: hp.String(),
	}, nil
}

func (hp *HostPort) UnmarshalYAML(n *yaml.Node) error {
	return hp.Set(n.Value)
}

func (*HostPort) Type() string {
	return "host:port"
}

var (
	_ yaml.Marshaler   = new(Struct[struct{}])
	_ yaml.Unmarshaler = new(Struct[struct{}])
)

// Struct is a special value type that encodes an arbitrary struct.
// It implements the flag.Value interface, but in general these values should
// only be accepted via config for ergonomics.
//
// The string encoding type is YAML.
type Struct[T any] struct {
	Value T
}

//nolint:revive
func (s *Struct[T]) Set(v string) error {
	return yaml.Unmarshal([]byte(v), &s.Value)
}

//nolint:revive
func (s *Struct[T]) String() string {
	byt, err := yaml.Marshal(s.Value)
	if err != nil {
		return "decode failed: " + err.Error()
	}
	return string(byt)
}

func (s *Struct[T]) MarshalYAML() (interface{}, error) {
	var n yaml.Node
	err := n.Encode(s.Value)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Struct[T]) UnmarshalYAML(n *yaml.Node) error {
	// HACK: for compatibility with flags, we use nil slices instead of empty
	// slices. In most cases, nil slices and empty slices are treated
	// the same, so this behavior may be removed at some point.
	if typ := reflect.TypeOf(s.Value); typ.Kind() == reflect.Slice && len(n.Content) == 0 {
		reflect.ValueOf(&s.Value).Elem().Set(reflect.Zero(typ))
		return nil
	}
	return n.Decode(&s.Value)
}

//nolint:revive
func (s *Struct[T]) Type() string {
	return fmt.Sprintf("struct[%T]", s.Value)
}

func (s *Struct[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Value)
}

func (s *Struct[T]) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &s.Value)
}

// DiscardValue does nothing but implements the pflag.Value interface.
// It's useful in cases where you want to accept an option, but access the
// underlying value directly instead of through the Option methods.
var DiscardValue discardValue

type discardValue struct{}

func (discardValue) Set(string) error {
	return nil
}

func (discardValue) String() string {
	return ""
}

func (discardValue) Type() string {
	return "discard"
}

func (discardValue) UnmarshalJSON([]byte) error {
	return nil
}

// jsonValue is intentionally not exported. It is just used to store the raw JSON
// data for a value to defer it's unmarshal. It implements the pflag.Value to be
// usable in an Option.
type jsonValue json.RawMessage

func (jsonValue) Set(string) error {
	return xerrors.Errorf("json value is read-only")
}

func (jsonValue) String() string {
	return ""
}

func (jsonValue) Type() string {
	return "json"
}

func (j *jsonValue) UnmarshalJSON(data []byte) error {
	if j == nil {
		return xerrors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

var _ pflag.Value = (*Enum)(nil)

type Enum struct {
	Choices []string
	Value   *string
}

func EnumOf(v *string, choices ...string) *Enum {
	return &Enum{
		Choices: choices,
		Value:   v,
	}
}

func (e *Enum) Set(v string) error {
	for _, c := range e.Choices {
		if v == c {
			*e.Value = v
			return nil
		}
	}
	return xerrors.Errorf("invalid choice: %s, should be one of %v", v, e.Choices)
}

func (e *Enum) Type() string {
	return fmt.Sprintf("enum[%v]", strings.Join(e.Choices, "\\|"))
}

func (e *Enum) String() string {
	return *e.Value
}

type Regexp regexp.Regexp

func (r *Regexp) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Regexp) UnmarshalJSON(data []byte) error {
	var source string
	err := json.Unmarshal(data, &source)
	if err != nil {
		return err
	}

	exp, err := regexp.Compile(source)
	if err != nil {
		return xerrors.Errorf("invalid regex expression: %w", err)
	}
	*r = Regexp(*exp)
	return nil
}

func (r *Regexp) MarshalYAML() (interface{}, error) {
	return yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: r.String(),
	}, nil
}

func (r *Regexp) UnmarshalYAML(n *yaml.Node) error {
	return r.Set(n.Value)
}

func (r *Regexp) Set(v string) error {
	exp, err := regexp.Compile(v)
	if err != nil {
		return xerrors.Errorf("invalid regex expression: %w", err)
	}
	*r = Regexp(*exp)
	return nil
}

func (r Regexp) String() string {
	return r.Value().String()
}

func (r *Regexp) Value() *regexp.Regexp {
	if r == nil {
		return nil
	}
	return (*regexp.Regexp)(r)
}

func (Regexp) Type() string {
	return "regexp"
}

var _ pflag.Value = (*YAMLConfigPath)(nil)

// YAMLConfigPath is a special value type that encodes a path to a YAML
// configuration file where options are read from.
type YAMLConfigPath string

func (p *YAMLConfigPath) Set(v string) error {
	*p = YAMLConfigPath(v)
	return nil
}

func (p *YAMLConfigPath) String() string {
	return string(*p)
}

func (*YAMLConfigPath) Type() string {
	return "yaml-config-path"
}
