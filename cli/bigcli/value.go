package bigcli

import "strconv"

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
