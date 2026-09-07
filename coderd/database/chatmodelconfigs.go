package database

// EffectiveChatModelConfigs contains enabled configs for one organization.
type EffectiveChatModelConfigs struct {
	Configs       []GetEnabledChatModelConfigsByOrganizationRow
	DefaultConfig ChatModelConfig
}

// DeriveEffectiveChatModelConfigs selects the stored default from enabled
// configs in one organization.
func DeriveEffectiveChatModelConfigs(
	rows []GetEnabledChatModelConfigsByOrganizationRow,
) EffectiveChatModelConfigs {
	result := EffectiveChatModelConfigs{Configs: rows}
	for _, row := range rows {
		if row.ChatModelConfig.IsDefault {
			result.DefaultConfig = row.ChatModelConfig
		}
	}
	return result
}
