import type {
	ChatModelConfig,
	ChatModelProvider,
	ChatProviderConfig,
} from "#/api/typesGenerated";
import { MOCK_TIMESTAMP } from "./chatEntities";

export const MockChatModelConfig: ChatModelConfig = {
	id: "model-1",
	ai_provider_id: "provider-1",
	model: "gpt-5",
	display_name: "gpt-5",
	enabled: true,
	is_default: false,
	context_limit: 200000,
	compression_threshold: 70,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockChatProviderConfig: ChatProviderConfig = {
	id: "provider-1",
	provider: "openai",
	display_name: "OpenAI",
	icon: "",
	enabled: true,
	has_api_key: true,
	central_api_key_enabled: true,
	allow_user_api_key: false,
	allow_central_api_key_fallback: true,
	base_url: "",
	source: "database",
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockChatModelProvider: ChatModelProvider = {
	provider: "openai",
	available: true,
	models: [],
};
