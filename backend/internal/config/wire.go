package config

import "github.com/google/wire"

// ProviderSet 提供配置层的依赖
var ProviderSet = wire.NewSet(
	ProvideConfig,
)

// ProvideConfig 提供应用配置
func ProvideConfig() (*Config, error) {
	cfg, err := LoadForBootstrap()
	if err != nil {
		return nil, err
	}
	if err := cfg.Security.URLAllowlist.loadUpstreamAllowlist(); err != nil {
		return nil, err
	}
	return cfg, nil
}
