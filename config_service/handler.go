package main

import (
	config "config_service/kitex_gen/config"
	"context"
)

// ConfigServiceImpl implements the last service interface defined in the IDL.
type ConfigServiceImpl struct{}

// GetConfig implements the ConfigServiceImpl interface.
func (s *ConfigServiceImpl) GetConfig(ctx context.Context, req *config.GetConfigRequest) (resp *config.GetConfigResponse, err error) {
	// 示例配置逻辑：版本大于1.0时启用Redis
	useRedis := req.Version > "1.0"
	return &config.GetConfigResponse{UseRedis: useRedis}, nil
}
