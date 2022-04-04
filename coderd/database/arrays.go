package database

import (
	"github.com/jackc/pgtype"
	"golang.org/x/xerrors"
)

type ProvisionerTypes []ProvisionerType

func (p *ProvisionerTypes) Set(src interface{}) error {
	switch vs := src.(type) {
	case []ProvisionerType:
		*p = make([]ProvisionerType, len(vs))
		for i, v := range vs {
			(*p)[i] = v
		}
	default:
		panic("what")
	}

	return nil
}

func (p *ProvisionerTypes) Get() interface{} {
	return []ProvisionerType(*p)
}

func (p *ProvisionerTypes) AssignTo(dst interface{}) error {
	switch vs := dst.(type) {
	case *[]ProvisionerType:
		*vs = make([]ProvisionerType, len(*p))
		for i, v := range *p {
			(*vs)[i] = v
		}
		return nil

	default:
		panic("what")
	}
}

func (p *ProvisionerTypes) DecodeText(ci *pgtype.ConnInfo, src []byte) error {
	if src == nil {
		return xerrors.New("NULL values can't be decoded. Scan into a &amp;*Positions to handle NULLs")
	}

	val := pgtype.EnumArray{}
	err := val.DecodeText(ci, src)
	if err != nil {
		return err
	}

	*p = ProvisionerTypes{}
	for i := 0; i < len(val.Elements); i++ {
		*p = append(*p, ProvisionerType(val.Elements[i].String))
	}

	return nil
}

func (p *ProvisionerTypes) EncodeText(ci *pgtype.ConnInfo, buf []byte) ([]byte, error) {
	if p == nil {
		return pgtype.EnumArray{Status: pgtype.Null}.EncodeText(ci, buf)
	}

	val := pgtype.EnumArray{Status: pgtype.Present}

	if len(*p) > 0 {
		val.Dimensions = []pgtype.ArrayDimension{{Length: int32(len(*p)), LowerBound: 1}}
	}

	for _, typ := range *p {
		val.Elements = append(val.Elements, pgtype.GenericText{
			String: string(typ),
			Status: pgtype.Present,
		})
	}

	return val.EncodeText(ci, buf)
}

type LogLevels []LogLevel

func (p *LogLevels) Set(src interface{}) error {
	switch vs := src.(type) {
	case []LogLevel:
		*p = make([]LogLevel, len(vs))
		for i, v := range vs {
			(*p)[i] = v
		}
	default:
		panic("what")
	}

	return nil
}

func (p *LogLevels) Get() interface{} {
	return []LogLevel(*p)
}

func (p *LogLevels) AssignTo(dst interface{}) error {
	switch vs := dst.(type) {
	case *[]LogLevel:
		*vs = make([]LogLevel, len(*p))
		for i, v := range *p {
			(*vs)[i] = v
		}
		return nil
	}

	dst = []LogLevel(*p)
	return nil
}

func (p *LogLevels) DecodeText(ci *pgtype.ConnInfo, src []byte) error {
	if src == nil {
		return xerrors.New("NULL values can't be decoded. Scan into a &amp;*Positions to handle NULLs")
	}

	val := pgtype.EnumArray{}
	err := val.DecodeText(ci, src)
	if err != nil {
		return err
	}

	*p = LogLevels{}
	for i := 0; i < len(val.Elements); i++ {
		*p = append(*p, LogLevel(val.Elements[i].String))
	}

	return nil
}

func (p *LogLevels) EncodeText(ci *pgtype.ConnInfo, buf []byte) ([]byte, error) {
	if p == nil {
		return pgtype.EnumArray{Status: pgtype.Null}.EncodeText(ci, buf)
	}

	val := pgtype.EnumArray{Status: pgtype.Present}

	if len(*p) > 0 {
		val.Dimensions = []pgtype.ArrayDimension{{Length: int32(len(*p)), LowerBound: 1}}
	}

	for _, typ := range *p {
		val.Elements = append(val.Elements, pgtype.GenericText{
			String: string(typ),
			Status: pgtype.Present,
		})
	}

	return val.EncodeText(ci, buf)
}

type LogSources []LogSource

func (p *LogSources) Set(src interface{}) error {
	switch vs := src.(type) {
	case []LogSource:
		*p = make([]LogSource, len(vs))
		for i, v := range vs {
			(*p)[i] = v
		}
	default:
		panic("what")
	}

	return nil
}

func (p *LogSources) Get() interface{} {
	return []LogSource(*p)
}

func (p *LogSources) AssignTo(dst interface{}) error {
	switch vs := dst.(type) {
	case *[]LogSource:
		*vs = make([]LogSource, len(*p))
		for i, v := range *p {
			(*vs)[i] = v
		}
		return nil
	}

	dst = []LogSource(*p)
	return nil
}

func (p *LogSources) DecodeText(ci *pgtype.ConnInfo, src []byte) error {
	if src == nil {
		return xerrors.New("NULL values can't be decoded. Scan into a &amp;*Positions to handle NULLs")
	}

	val := pgtype.EnumArray{}
	err := val.DecodeText(ci, src)
	if err != nil {
		return err
	}

	*p = LogSources{}
	for i := 0; i < len(val.Elements); i++ {
		*p = append(*p, LogSource(val.Elements[i].String))
	}

	return nil
}

func (p *LogSources) EncodeText(ci *pgtype.ConnInfo, buf []byte) ([]byte, error) {
	if p == nil {
		return pgtype.EnumArray{Status: pgtype.Null}.EncodeText(ci, buf)
	}

	val := pgtype.EnumArray{Status: pgtype.Present}

	if len(*p) > 0 {
		val.Dimensions = []pgtype.ArrayDimension{{Length: int32(len(*p)), LowerBound: 1}}
	}

	for _, typ := range *p {
		val.Elements = append(val.Elements, pgtype.GenericText{
			String: string(typ),
			Status: pgtype.Present,
		})
	}

	return val.EncodeText(ci, buf)
}
