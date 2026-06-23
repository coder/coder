import type * as TypesGen from "#/api/typesGenerated";

const SECRET_PLACEHOLDER = "••••••••••••••••";

export const TRANSPORT_OPTIONS = [
	{ value: "streamable_http", label: "Streamable HTTP" },
	{ value: "sse", label: "SSE" },
] as const;

export const AUTH_TYPE_OPTIONS = [
	{ value: "none", label: "None" },
	{ value: "oauth2", label: "OAuth2" },
	{ value: "api_key", label: "API key" },
	{ value: "custom_headers", label: "Custom headers" },
	{ value: "user_oidc", label: "User OIDC identity" },
] as const;

export const AVAILABILITY_OPTIONS = [
	{
		value: "force_on",
		label: "Force on",
		description: "Always injected into every conversation.",
	},
	{
		value: "default_on",
		label: "Default on",
		description: "Pre-selected but users can opt out.",
	},
	{
		value: "default_off",
		label: "Default off",
		description: "Available but users must opt in.",
	},
] as const;

export interface MCPServerFormValues {
	displayName: string;
	slug: string;
	slugTouched: boolean;
	description: string;
	iconURL: string;
	url: string;
	transport: string;
	authType: string;
	oauth2ClientID: string;
	oauth2ClientSecret: string;
	oauth2SecretTouched: boolean;
	oauth2AuthURL: string;
	oauth2TokenURL: string;
	oauth2Scopes: string;
	apiKeyHeader: string;
	apiKeyValue: string;
	apiKeyTouched: boolean;
	availability: string;
	enabled: boolean;
	modelIntent: boolean;
	allowInPlanMode: boolean;
	forwardCoderHeaders: boolean;
	toolAllowList: string;
	toolDenyList: string;
	customHeaders: Array<{ key: string; value: string }>;
	customHeadersTouched: boolean;
}

export const slugify = (value: string): string =>
	value
		.toLowerCase()
		.trim()
		.replace(/[^a-z0-9-]+/g, "-")
		.replace(/^-+|-+$/g, "");

const splitList = (value: string): string[] =>
	value
		.split(",")
		.map((item) => item.trim())
		.filter(Boolean);

const joinList = (arr: readonly string[] | undefined): string =>
	arr?.join(", ") ?? "";

export const authTypeLabel = (value: string): string =>
	AUTH_TYPE_OPTIONS.find((option) => option.value === value)?.label ?? value;

export const availabilityLabel = (value: string): string =>
	AVAILABILITY_OPTIONS.find((option) => option.value === value)?.label ?? value;

export const buildInitialValues = (
	server?: TypesGen.MCPServerConfig,
): MCPServerFormValues => ({
	displayName: server?.display_name ?? "",
	slug: server?.slug ?? "",
	slugTouched: false,
	description: server?.description ?? "",
	iconURL: server?.icon_url ?? "",
	url: server?.url ?? "",
	transport: server?.transport ?? "streamable_http",
	authType: server?.auth_type ?? "none",
	oauth2ClientID: server?.oauth2_client_id ?? "",
	oauth2ClientSecret: server?.has_oauth2_secret ? SECRET_PLACEHOLDER : "",
	oauth2SecretTouched: false,
	oauth2AuthURL: server?.oauth2_auth_url ?? "",
	oauth2TokenURL: server?.oauth2_token_url ?? "",
	oauth2Scopes: server?.oauth2_scopes ?? "",
	apiKeyHeader: server?.api_key_header ?? "",
	apiKeyValue: server?.has_api_key ? SECRET_PLACEHOLDER : "",
	apiKeyTouched: false,
	availability: server?.availability ?? "default_off",
	enabled: server?.enabled ?? true,
	modelIntent: server?.model_intent ?? false,
	allowInPlanMode: server?.allow_in_plan_mode ?? false,
	forwardCoderHeaders: server?.forward_coder_headers ?? false,
	toolAllowList: joinList(server?.tool_allow_list),
	toolDenyList: joinList(server?.tool_deny_list),
	customHeaders: [],
	customHeadersTouched: false,
});

export const canSubmitMCPServerForm = (
	values: MCPServerFormValues,
	isDisabled: boolean,
): boolean =>
	values.displayName.trim() !== "" &&
	values.slug.trim() !== "" &&
	values.url.trim() !== "" &&
	!isDisabled;

export const buildCreateRequest = (
	values: MCPServerFormValues,
): TypesGen.CreateMCPServerConfigRequest => {
	const effectiveOAuth2Secret =
		values.oauth2SecretTouched &&
		values.oauth2ClientSecret !== SECRET_PLACEHOLDER
			? values.oauth2ClientSecret
			: undefined;
	const effectiveApiKeyValue =
		values.apiKeyTouched && values.apiKeyValue !== SECRET_PLACEHOLDER
			? values.apiKeyValue
			: undefined;

	return {
		display_name: values.displayName.trim(),
		slug: values.slug.trim(),
		description: values.description.trim(),
		icon_url: values.iconURL.trim(),
		url: values.url.trim(),
		transport: values.transport,
		auth_type: values.authType,
		availability: values.availability,
		enabled: values.enabled,
		model_intent: values.modelIntent,
		allow_in_plan_mode: values.allowInPlanMode,
		forward_coder_headers: values.forwardCoderHeaders,
		...(values.authType === "oauth2" && {
			oauth2_client_id: values.oauth2ClientID.trim(),
			oauth2_client_secret: effectiveOAuth2Secret,
			oauth2_auth_url: values.oauth2AuthURL.trim() || undefined,
			oauth2_token_url: values.oauth2TokenURL.trim() || undefined,
			oauth2_scopes: values.oauth2Scopes.trim() || undefined,
		}),
		...(values.authType === "api_key" && {
			api_key_header: values.apiKeyHeader.trim() || undefined,
			api_key_value: effectiveApiKeyValue,
		}),
		...(values.authType === "custom_headers" &&
			values.customHeadersTouched && {
				custom_headers: Object.fromEntries(
					values.customHeaders
						.filter((header) => header.key.trim() !== "")
						.map((header) => [header.key.trim(), header.value]),
				),
			}),
		tool_allow_list: splitList(values.toolAllowList),
		tool_deny_list: splitList(values.toolDenyList),
	};
};

export const buildUpdateRequest = (
	values: MCPServerFormValues,
): TypesGen.UpdateMCPServerConfigRequest => {
	const req = buildCreateRequest(values);
	return {
		...req,
		tool_allow_list: req.tool_allow_list ? [...req.tool_allow_list] : undefined,
		tool_deny_list: req.tool_deny_list ? [...req.tool_deny_list] : undefined,
	};
};
