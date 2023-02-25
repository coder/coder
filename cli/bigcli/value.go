package bigcli

import (
	"net/url"
	"strconv"
	"time"
)

// values.go contains a standard set of value types that can be used as
// Option Values.

type Int int

func (i *Int) Set(s string) error {
	ii, err := strconv.ParseInt(s, 10, 64)
	*i = Int(ii)
	return err
}

func (i Int) String() string {
	return strconv.Itoa(int(i))
}

func (Int) Type() string {
	return "int"
}

type String string

func (s *String) Set(v string) error {
	*s = String(v)
	return nil
}

func (s String) String() string {
	return string(s)
}

func (String) Type() string {
	return "string"
}

type Duration time.Duration

func (d *Duration) Set(v string) error {
	dd, err := time.ParseDuration(v)
	*d = Duration(dd)
	return err
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

func (u *URL) URL() *url.URL {
	return (*url.URL)(u)
}
