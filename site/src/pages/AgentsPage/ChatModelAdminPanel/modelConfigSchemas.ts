import type * as TypesGen from "api/typesGenerated";
import type { ProviderState } from "./ChatModelAdminPanel";

type ProviderModelConfigSchemaReference = {
	modelConfig: TypesGen.ChatModelCallConfig;
	notes?: readonly string[];
};

const modelConfigSchemaByProvider: Record<
	string,
	ProviderModelConfigSchemaReference
> = {
	openai: {
		modelConfig: {
			max_output_tokens: 32000,
			temperature: 0.2,
			top_p: 0.95,
			top_k: 40,
			presence_penalty: 0,
			frequency_penalty: 0,
			provider_options: {
				openai: {
					reasoning_effort: "high",
					parallel_tool_calls: true,
					text_verbosity: "low",
					service_tier: "auto",
					user: "end-user-id",
				},
			},
		},
		notes: ["Responses API models may also use reasoning_summary and include."],
	},
	azure: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				openai: {
					reasoning_effort: "high",
					parallel_tool_calls: true,
					user: "end-user-id",
				},
			},
		},
		notes: ["Azure uses OpenAI provider option keys in Fantasy."],
	},
	anthropic: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				anthropic: {
					effort: "medium",
					thinking: { budget_tokens: 4000 },
					send_reasoning: true,
					disable_parallel_tool_use: false,
				},
			},
		},
	},
	bedrock: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				anthropic: {
					effort: "medium",
					thinking: { budget_tokens: 4000 },
					send_reasoning: true,
					disable_parallel_tool_use: false,
				},
			},
		},
		notes: ["Bedrock uses Anthropic option keys in Fantasy."],
	},
	google: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				google: {
					thinking_config: {
						thinking_budget: 1024,
						include_thoughts: true,
					},
					safety_settings: [
						{
							category: "HARM_CATEGORY_DANGEROUS_CONTENT",
							threshold: "BLOCK_ONLY_HIGH",
						},
					],
				},
			},
		},
	},
	openaicompat: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				openaicompat: {
					reasoning_effort: "medium",
					user: "end-user-id",
				},
			},
		},
	},
	openrouter: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				openrouter: {
					reasoning: {
						enabled: true,
						effort: "medium",
						max_tokens: 2048,
						exclude: false,
					},
					parallel_tool_calls: true,
					include_usage: true,
					user: "end-user-id",
				},
			},
		},
	},
	vercel: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				vercel: {
					reasoning: {
						enabled: true,
						effort: "medium",
						max_tokens: 2048,
						exclude: false,
					},
					parallel_tool_calls: true,
					user: "end-user-id",
				},
			},
		},
	},
};

export const getModelConfigSchemaReference = (
	providerState: ProviderState | null,
) => {
	const providerLabel = providerState?.label ?? "Provider";
	const normalizedProvider = (providerState?.provider ?? "")
		.trim()
		.toLowerCase();
	const providerConfigSchema = modelConfigSchemaByProvider[normalizedProvider];
	const modelConfigTemplate = providerConfigSchema?.modelConfig ?? {};
	const notes = providerConfigSchema?.notes ?? [
		"No provider-specific options are documented for this provider yet.",
	];

	const schema: TypesGen.CreateChatModelConfigRequest = {
		provider: normalizedProvider || "<provider>",
		model: "<model-id>",
		context_limit: 200000,
		compression_threshold: 70,
		model_config: modelConfigTemplate,
	};

	return {
		providerLabel,
		notes,
		schemaJSON: JSON.stringify(schema, null, 2),
	};
};
