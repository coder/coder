package deployment

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/coder/coder/codersdk"
)

func DefaultViper() *viper.Viper {
	v := viper.New()
	v.SetDefault("access_url", "")

	return v
}

func AttachFlags(flagset *pflag.FlagSet, vip *viper.Viper) {
	_ = flagset.StringP("access-url", "", vip.GetString("access-url"), "usage")
	_ = vip.BindPFlag("access-url", flagset.Lookup("access-url"))
}

func AttachEnterpriseFlags(flagset *pflag.FlagSet, vip *viper.Viper) {
	_ = flagset.StringP("access-url", "", vip.GetString("access-url"), "usage")
	_ = vip.BindPFlag("access-url", flagset.Lookup("access-url"))
}

func Config(vip *viper.Viper) (codersdk.DeploymentConfig, error) {
	cfg := codersdk.DeploymentConfig{}
	return cfg, vip.Unmarshal(cfg)
}
