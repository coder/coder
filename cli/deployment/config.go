package deployment

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
)

//nolint:revive
func Config(flagset *pflag.FlagSet, vip *viper.Viper) (*codersdk.DeploymentConfig, error) {
	dc := newConfig()
	flg, err := flagset.GetString(config.FlagName)
	if err != nil {
		return nil, xerrors.Errorf("get global config from flag: %w", err)
	}
	vip.SetEnvPrefix("coder")

	if flg != "" {
		vip.SetConfigFile(flg + "/server.yaml")
		err = vip.ReadInConfig()
		if err != nil && !xerrors.Is(err, os.ErrNotExist) {
			return dc, xerrors.Errorf("reading deployment config: %w", err)
		}
	}

	setConfig("", vip, &dc)

	return dc, nil
}

func setConfig(prefix string, vip *viper.Viper, target interface{}) {
	val := reflect.Indirect(reflect.ValueOf(target))
	typ := val.Type()
	if typ.Kind() != reflect.Struct {
		val = val.Elem()
		typ = val.Type()
	}

	// Ensure that we only bind env variables to proper fields,
	// otherwise Viper will get confused if the parent struct is
	// assigned a value.
	if strings.HasPrefix(typ.Name(), "DeploymentConfigField[") {
		value := val.FieldByName("Value").Interface()

		env, ok := val.FieldByName("EnvOverride").Interface().(string)
		if !ok {
			panic("DeploymentConfigField[].EnvOverride must be a string")
		}
		if env == "" {
			env = formatEnv(prefix)
		}

		switch value.(type) {
		case string:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetString(vip.GetString(prefix))
		case bool:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetBool(vip.GetBool(prefix))
		case int:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetInt(int64(vip.GetInt(prefix)))
		case time.Duration:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetInt(int64(vip.GetDuration(prefix)))
		case []string:
			vip.MustBindEnv(prefix, env)
			// As of October 21st, 2022 we supported delimiting a string
			// with a comma, but Viper only supports with a space. This
			// is a small hack around it!
			rawSlice := reflect.ValueOf(vip.GetStringSlice(prefix)).Interface()
			stringSlice, ok := rawSlice.([]string)
			if !ok {
				panic(fmt.Sprintf("string slice is of type %T", rawSlice))
			}
			value := make([]string, 0, len(stringSlice))
			for _, entry := range stringSlice {
				value = append(value, strings.Split(entry, ",")...)
			}
			val.FieldByName("Value").Set(reflect.ValueOf(value))
		case []codersdk.GitAuthConfig:
			// Do not bind to CODER_GITAUTH, instead bind to CODER_GITAUTH_0_*, etc.
			values := readSliceFromViper[codersdk.GitAuthConfig](vip, prefix, value)
			val.FieldByName("Value").Set(reflect.ValueOf(values))
		default:
			panic(fmt.Sprintf("unsupported type %T", value))
		}
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		fv := val.Field(i)
		ft := fv.Type()
		if isBigCLI(ft) {
			// Ignore these values while migrating.
			continue
		}
		tag := typ.Field(i).Tag.Get("json")
		var key string
		if prefix == "" {
			key = tag
		} else {
			key = fmt.Sprintf("%s.%s", prefix, tag)
		}
		switch ft.Kind() {
		case reflect.Ptr:
			setConfig(key, vip, fv.Interface())
		case reflect.Slice:
			for j := 0; j < fv.Len(); j++ {
				key := fmt.Sprintf("%s.%d", key, j)
				setConfig(key, vip, fv.Index(j).Interface())
			}
		default:
			panic(fmt.Sprintf("unsupported type %T", ft))
		}
	}
}

// readSliceFromViper reads a typed mapping from the key provided.
// This enables environment variables like CODER_GITAUTH_<index>_CLIENT_ID.
func readSliceFromViper[T any](vip *viper.Viper, key string, value any) []T {
	elementType := reflect.TypeOf(value).Elem()
	returnValues := make([]T, 0)
	for entry := 0; true; entry++ {
		// Only create an instance when the entry exists in viper...
		// otherwise we risk
		var instance *reflect.Value
		for i := 0; i < elementType.NumField(); i++ {
			fve := elementType.Field(i)
			prop := fve.Tag.Get("json")
			// For fields that are omitted in JSON, we use a YAML tag.
			if prop == "-" {
				prop = fve.Tag.Get("yaml")
			}
			configKey := fmt.Sprintf("%s.%d.%s", key, entry, prop)

			// Ensure the env entry for this key is registered
			// before checking value.
			//
			// We don't support DeploymentConfigField[].EnvOverride for array flags so
			// this is fine to just use `formatEnv` here.
			vip.MustBindEnv(configKey, formatEnv(configKey))

			value := vip.Get(configKey)
			if value == nil {
				continue
			}
			if instance == nil {
				newType := reflect.Indirect(reflect.New(elementType))
				instance = &newType
			}
			switch v := instance.Field(i).Type().String(); v {
			case "[]string":
				value = vip.GetStringSlice(configKey)
			case "bool":
				value = vip.GetBool(configKey)
			default:
			}
			instance.Field(i).Set(reflect.ValueOf(value))
		}
		if instance == nil {
			break
		}
		value, ok := instance.Interface().(T)
		if !ok {
			continue
		}
		returnValues = append(returnValues, value)
	}
	return returnValues
}

func NewViper() *viper.Viper {
	dc := newConfig()
	vip := viper.New()
	vip.SetEnvPrefix("coder")
	vip.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	setViperDefaults("", vip, dc)

	return vip
}

func isBigCLI(typ reflect.Type) bool {
	return strings.Contains(typ.PkgPath(), "bigcli")
}

func setViperDefaults(prefix string, vip *viper.Viper, target interface{}) {
	val := reflect.ValueOf(target).Elem()
	val = reflect.Indirect(val)
	typ := val.Type()
	if strings.HasPrefix(typ.Name(), "DeploymentConfigField[") {
		value := val.FieldByName("Default").Interface()
		vip.SetDefault(prefix, value)
		return
	}

	if isBigCLI(typ) {
		// Ignore these values while migrating.
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		fv := val.Field(i)
		ft := fv.Type()
		if isBigCLI(ft) {
			// Ignore these values while migrating.
			continue
		}
		tag := typ.Field(i).Tag.Get("json")
		var key string
		if prefix == "" {
			key = tag
		} else {
			key = fmt.Sprintf("%s.%s", prefix, tag)
		}
		switch ft.Kind() {
		case reflect.Ptr:
			setViperDefaults(key, vip, fv.Interface())
		case reflect.Slice:
			// we currently don't support default values on structured slices
			continue
		default:
			panic(fmt.Sprintf("unsupported type %v", ft.String()))
		}
	}
}

//nolint:revive
func AttachFlags(flagset *pflag.FlagSet, vip *viper.Viper, enterprise bool) {
	setFlags("", flagset, vip, newConfig(), enterprise)
}

//nolint:revive
func setFlags(prefix string, flagset *pflag.FlagSet, vip *viper.Viper, target interface{}, enterprise bool) {
	val := reflect.Indirect(reflect.ValueOf(target))
	typ := val.Type()
	if strings.HasPrefix(typ.Name(), "DeploymentConfigField[") {
		isEnt := val.FieldByName("Enterprise").Bool()
		if enterprise != isEnt {
			return
		}
		flg := val.FieldByName("Flag").String()
		if flg == "" {
			return
		}

		env, ok := val.FieldByName("EnvOverride").Interface().(string)
		if !ok {
			panic("DeploymentConfigField[].EnvOverride must be a string")
		}
		if env == "" {
			env = formatEnv(prefix)
		}

		usage := val.FieldByName("Usage").String()
		usage = fmt.Sprintf("%s\n%s", usage, cliui.Styles.Placeholder.Render("Consumes $"+env))
		shorthand := val.FieldByName("Shorthand").String()
		hidden := val.FieldByName("Hidden").Bool()
		value := val.FieldByName("Default").Interface()

		// Allow currently set environment variables
		// to override default values in help output.
		vip.MustBindEnv(prefix, env)

		switch value.(type) {
		case string:
			_ = flagset.StringP(flg, shorthand, vip.GetString(prefix), usage)
		case bool:
			_ = flagset.BoolP(flg, shorthand, vip.GetBool(prefix), usage)
		case int:
			_ = flagset.IntP(flg, shorthand, vip.GetInt(prefix), usage)
		case time.Duration:
			_ = flagset.DurationP(flg, shorthand, vip.GetDuration(prefix), usage)
		case []string:
			_ = flagset.StringSliceP(flg, shorthand, vip.GetStringSlice(prefix), usage)
		case []codersdk.GitAuthConfig:
			// Ignore this one!
		default:
			panic(fmt.Sprintf("unsupported type %T", typ))
		}

		_ = vip.BindPFlag(prefix, flagset.Lookup(flg))
		if hidden {
			_ = flagset.MarkHidden(flg)
		}

		return
	}

	for i := 0; i < typ.NumField(); i++ {
		fv := val.Field(i)
		ft := fv.Type()
		if isBigCLI(ft) {
			// Ignore these values while migrating.
			continue
		}
		tag := typ.Field(i).Tag.Get("json")
		var key string
		if prefix == "" {
			key = tag
		} else {
			key = fmt.Sprintf("%s.%s", prefix, tag)
		}
		switch ft.Kind() {
		case reflect.Ptr:
			setFlags(key, flagset, vip, fv.Interface(), enterprise)
		case reflect.Slice:
			for j := 0; j < fv.Len(); j++ {
				key := fmt.Sprintf("%s.%d", key, j)
				setFlags(key, flagset, vip, fv.Index(j).Interface(), enterprise)
			}
		default:
			panic(fmt.Sprintf("unsupported type %v", ft))
		}
	}
}

func formatEnv(key string) string {
	return "CODER_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(key))
}
