package bigcli

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
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

func (b *Bool) Set(s string) error {
	if s == "" {
		*b = Bool(false)
		return nil
	}
	bb, err := strconv.ParseBool(s)
	*b = Bool(bb)
	return err
}

func (b *Bool) NoOptDefValue() string {
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

type Strings []string

func (s *Strings) Set(v string) error {
	*s = strings.Split(v, ",")
	return nil
}

func (s Strings) String() string {
	return strings.Join(s, ",")
}

func (s Strings) Value() []string {
	return []string(s)
}

func (Strings) Type() string {
	return "strings"
}

type Duration time.Duration

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

type URL url.URL

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

func (*URL) Type() string {
	return "url"
}

func (u *URL) Value() *url.URL {
	return (*url.URL)(u)
}

type BindAddress struct {
	Host string
	Port string
}

func (b *BindAddress) Set(v string) error {
	var err error
	b.Host, b.Port, err = net.SplitHostPort(v)
	return err
}

func (b *BindAddress) String() string {
	if b.Host == "" && b.Port == "" {
		return ""
	}
	return b.Host + ":" + b.Port
}

func (*BindAddress) Type() string {
	return "bind-address"
}

type DiscardValue struct{}

func (DiscardValue) Set(string) error {
	return nil
}

func (DiscardValue) String() string {
	return ""
}

func (DiscardValue) Type() string {
	return "nop"
}
