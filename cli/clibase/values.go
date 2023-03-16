package clibase

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
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

var _ pflag.SliceValue = &Strings{}

// Strings is a slice of strings that implements pflag.Value and pflag.SliceValue.
type Strings []string

func StringsOf(ss *[]string) *Strings {
	return (*Strings)(ss)
}

func (s *Strings) Append(v string) error {
	*s = append(*s, v)
	return nil
}

func (s *Strings) Replace(vals []string) error {
	*s = vals
	return nil
}

func (s *Strings) GetSlice() []string {
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

func (s *Strings) Set(v string) error {
	ss, err := readAsCSV(v)
	if err != nil {
		return err
	}
	*s = append(*s, ss...)
	return nil
}

func (s Strings) String() string {
	return writeAsCSV([]string(s))
}

func (s Strings) Value() []string {
	return []string(s)
}

func (Strings) Type() string {
	return "strings"
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

func (d *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	return d.Set(s)
}

func (Duration) Type() string {
	return "duration"
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

func (*HostPort) Type() string {
	return "bind-address"
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

func (s *Struct[T]) Set(v string) error {
	return yaml.Unmarshal([]byte(v), &s.Value)
}

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
	return n.Decode(&s.Value)
}

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
	return fmt.Sprintf("enum[%v]", strings.Join(e.Choices, "|"))
}

func (e *Enum) String() string {
	return *e.Value
}
