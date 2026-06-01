import type {
	AIProvider,
	AIProviderBedrockSettings,
	AIProviderKeyMutation,
	AIProviderSettings,
	AIProviderType,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";
import {
	type ProviderFormValues,
	parseBedrockRegionFromBaseUrl,
	SAVED_CREDENTIAL_MASK,
} from "./ProviderForm";

/** Drop placeholder masks so they don't round-trip back to the API. */
const sanitizeCredential = (
	value: string,
	...extraMasks: (string | undefined)[]
): string => {
	const trimmed = value.trim();
	if (trimmed === "" || trimmed === SAVED_CREDENTIAL_MASK) {
		return "";
	}
	if (extraMasks.some((m) => m !== undefined && m === trimmed)) {
		return "";
	}
	return trimmed;
};

// The generated `AIProviderSettings` interface is empty (the Go side uses
// a custom marshaler), so we redeclare the structural wire shape here.
const BEDROCK_SETTINGS_TYPE = "bedrock";
const BEDROCK_SETTINGS_VERSION = 1;

type BedrockSettingsWire = AIProviderBedrockSettings & {
	_type: typeof BEDROCK_SETTINGS_TYPE;
	_version: typeof BEDROCK_SETTINGS_VERSION;
};

type SettingsWire = AIProviderSettings &
	Partial<AIProviderBedrockSettings> & {
		_type?: string;
		_version?: number;
	};

// Bedrock providers carry an Anthropic wire type plus a
// `settings._type === "bedrock"` discriminator. `settings` is non-null in
// the generated type but Go serializes zero settings as JSON `null`, so we
// null-check before reading the discriminator.
export const isBedrockProvider = (provider: AIProvider): boolean => {
	if (provider.type !== "anthropic") {
		return false;
	}
	const s = provider.settings as SettingsWire | null;
	return s !== null && s._type === BEDROCK_SETTINGS_TYPE;
};

export const hasBedrockStoredCredentials = (provider: AIProvider): boolean => {
	if (!isBedrockProvider(provider)) {
		return false;
	}
	// Bedrock secrets are write-only. The server only persists Bedrock
	// settings if credentials were supplied, so presence implies "on file".
	return true;
};

const parseProviderHost = (url: string): string => {
	try {
		return new URL(url).host.toLowerCase();
	} catch {
		return "";
	}
};

// UI types we recover from a saved provider's base_url because the wire
// `type` collapses them to `openai`. Matches the bare domain or any
// subdomain (Azure ships per-resource subdomains).
const displayTypeHosts: ReadonlyArray<[string, AIProviderType]> = [
	["openai.azure.com", "azure"],
	["generativelanguage.googleapis.com", "google"],
	["openrouter.ai", "openrouter"],
	["ai-gateway.vercel.sh", "vercel"],
];

const matchesHost = (host: string, suffix: string): boolean =>
	host === suffix || host.endsWith(`.${suffix}`);

// Wire `type` collapses azure/google/openrouter/vercel to `openai`, so
// we recover the original choice from the saved host. Bedrock comes
// through the settings discriminator. Unknown hosts fall back to wire.
export const getProviderDisplayType = (
	provider: AIProvider,
): AIProviderType => {
	if (isBedrockProvider(provider)) {
		return "bedrock";
	}
	if (provider.type === "anthropic") {
		return "anthropic";
	}
	const host = parseProviderHost(provider.base_url ?? "");
	const match = displayTypeHosts.find(([h]) => matchesHost(host, h));
	return match?.[1] ?? provider.type;
};

const buildBedrockSettings = (
	region: string | undefined,
	model: string,
	smallFastModel: string,
	accessKey: string,
	accessKeySecret: string,
): BedrockSettingsWire => ({
	_type: BEDROCK_SETTINGS_TYPE,
	_version: BEDROCK_SETTINGS_VERSION,
	...(region ? { region } : {}),
	model,
	small_fast_model: smallFastModel,
	...(accessKey ? { access_key: accessKey } : {}),
	...(accessKeySecret ? { access_key_secret: accessKeySecret } : {}),
});

// Bedrock credentials live in `settings`; openai/anthropic keys go in
// `api_keys`. `display_name` is omitted when blank so the server stores
// NULL and the UI falls back to `name`.
export const providerFormValuesToCreate = (
	values: ProviderFormValues,
): CreateAIProviderRequest => {
	const name = values.name.trim();
	const displayName = values.displayName.trim();
	const baseUrl = values.baseUrl.trim();

	if (values.type === "bedrock") {
		const region = parseBedrockRegionFromBaseUrl(baseUrl);
		const settings = buildBedrockSettings(
			region,
			values.model.trim(),
			values.smallFastModel.trim(),
			sanitizeCredential(values.accessKey),
			sanitizeCredential(values.accessKeySecret),
		);
		return {
			type: "anthropic",
			name,
			...(displayName ? { display_name: displayName } : {}),
			base_url: baseUrl,
			enabled: values.enabled,
			settings: settings as AIProviderSettings,
		};
	}

	// Copilot is a distinct wire type with no stored credential; the
	// runtime authenticates each request with the user's GitHub token.
	if (values.type === "copilot") {
		return {
			type: "copilot",
			name,
			...(displayName ? { display_name: displayName } : {}),
			base_url: baseUrl,
			enabled: values.enabled,
		};
	}

	const apiKey = sanitizeCredential(values.apiKey);
	// `""` is unreachable here (Yup blocks it, Bedrock branched out), but the
	// union still includes it; narrow so TS stays honest.
	if (values.type === "") {
		throw new Error("provider type is required");
	}
	// Wire only accepts `openai` and `anthropic`; the other UI types are
	// presets that collapse to `openai`.
	const wireType: AIProvider["type"] =
		values.type === "anthropic" ? "anthropic" : "openai";
	return {
		type: wireType,
		name,
		...(displayName ? { display_name: displayName } : {}),
		base_url: baseUrl,
		enabled: values.enabled,
		...(apiKey ? { api_keys: [apiKey] } : {}),
	};
};

// Bedrock secrets follow an "empty = keep" contract: blank inputs are
// omitted and the server leaves them unchanged. OpenAI/Anthropic keys ship
// as a declarative list: `{ id }` retains a saved key, `{ api_key }` inserts
// a new one, and any saved id missing from the list is deleted.
export const providerFormValuesToUpdate = (
	values: ProviderFormValues,
	existingProvider: AIProvider,
): UpdateAIProviderRequest => {
	const base: UpdateAIProviderRequest = {
		display_name: values.displayName.trim(),
		enabled: values.enabled,
		base_url: values.baseUrl.trim(),
	};

	// Copilot carries no stored credential, so a patch only touches the
	// base fields; the backend rejects api_keys on copilot providers.
	if (values.type === "copilot") {
		return base;
	}

	if (values.type !== "bedrock") {
		// If the user didn't touch the input, the form still holds the seeded
		// mask and sanitizes to `""` (no rotation).
		const savedMasked = existingProvider.api_keys[0]?.masked;
		const newApiKey = sanitizeCredential(values.apiKey, savedMasked);
		// Rotation goes out as the new plaintext alone: the saved key's id is
		// omitted (which deletes it) and the plaintext is inserted as a fresh
		// row. The backend rejects sending both fields on the same entry today.
		const apiKeys: AIProviderKeyMutation[] =
			newApiKey === ""
				? existingProvider.api_keys.map((k) => ({ id: k.id }))
				: [{ api_key: newApiKey }];
		return { ...base, api_keys: apiKeys };
	}

	const newAccessKey = sanitizeCredential(values.accessKey);
	const newAccessKeySecret = sanitizeCredential(values.accessKeySecret);
	// Yup enforces "both keys together"; if both survived the mask filter,
	// the user is rotating credentials.
	const credentialsChanged = newAccessKey !== "" && newAccessKeySecret !== "";

	// Yup blocks non-canonical Bedrock URLs upstream, so any `undefined`
	// region here is a real bug that should surface, not be papered over.
	const region = parseBedrockRegionFromBaseUrl(base.base_url ?? "");

	const settings = buildBedrockSettings(
		region,
		values.model.trim(),
		values.smallFastModel.trim(),
		credentialsChanged ? newAccessKey : "",
		credentialsChanged ? newAccessKeySecret : "",
	);

	return { ...base, settings: settings as AIProviderSettings };
};

// `name` is immutable on the server and the edit form hides it; we seed
// it anyway so the form values stay aligned with `ProviderFormValues`.
// `displayName` falls back to the slug for providers that never had one set.
export const aiProviderToFormValues = (
	provider: AIProvider,
): Partial<ProviderFormValues> => {
	const displayName = provider.display_name || provider.name;
	if (isBedrockProvider(provider)) {
		const s = (provider.settings as SettingsWire | null) ?? {};
		return {
			type: "bedrock",
			name: provider.name,
			displayName,
			baseUrl: provider.base_url,
			model: s.model ?? "",
			smallFastModel: s.small_fast_model ?? "",
			accessKey: "",
			accessKeySecret: "",
			enabled: provider.enabled,
		};
	}

	// Copilot is a distinct wire type whose form omits the credential
	// field, so it must be recovered exactly rather than collapsing to
	// openai.
	if (provider.type === "copilot") {
		return {
			type: "copilot",
			name: provider.name,
			displayName,
			baseUrl: provider.base_url,
			enabled: provider.enabled,
		};
	}

	// Wire `type` is otherwise only `openai` or `anthropic`; the dropdown's
	// richer labels apply only on create.
	return {
		type: provider.type === "anthropic" ? "anthropic" : "openai",
		name: provider.name,
		displayName,
		baseUrl: provider.base_url,
		apiKey: "",
		enabled: provider.enabled,
	};
};
