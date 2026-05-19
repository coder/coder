import type {
	AIProvider,
	AIProviderBedrockSettings,
	AIProviderKeyMutation,
	AIProviderSettings,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";
import { type ProviderFormValues, SAVED_CREDENTIAL_MASK } from "./ProviderForm";

/**
 * Treat the saved-credential mask the same as an empty value: never round-trip
 * the placeholder back to the API. Accepts an optional list of extra mask
 * strings (e.g. the API-supplied `provider.api_keys[0].masked` rendering for
 * openai/anthropic) so the dynamic mask is filtered the same way.
 */
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

/**
 * Server-side `AIProviderSettings` is a discriminated container that on the
 * wire flattens to a `{_type, _version, ...variantFields}` object. The
 * `codersdk` Go type uses a custom marshaler so the generated TypeScript
 * interface is empty; this is the structural shape we actually encode and
 * decode against.
 */
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

/**
 * The wire API only accepts `openai` and `anthropic` for the user-facing
 * create endpoint; AWS Bedrock is an Anthropic provider with
 * `settings._type === "bedrock"`.
 *
 * `AIProvider.settings` is typed as a non-null object by `typesGenerated.ts`,
 * but the Go side serializes the zero settings as JSON `null` (see
 * `AIProviderSettings.MarshalJSON`); we have to null-check before reading
 * any discriminator fields.
 */
export const isBedrockProvider = (provider: AIProvider): boolean => {
	if (provider.type !== "anthropic") {
		return false;
	}
	const s = provider.settings as SettingsWire | null;
	return s !== null && s._type === BEDROCK_SETTINGS_TYPE;
};

/** Bedrock has stored credentials on the server. */
export const hasBedrockStoredCredentials = (provider: AIProvider): boolean => {
	if (!isBedrockProvider(provider)) {
		return false;
	}
	// Bedrock secret fields are write-only and never present in responses, so
	// we can't observe the values directly. The server only persists Bedrock
	// settings if credentials were supplied, so the presence of a Bedrock
	// configuration implies credentials are on file.
	return true;
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

/**
 * Build a create request from form values. For Bedrock the API key field is
 * ignored; AWS credentials go into `settings`. For openai/anthropic the
 * plaintext API key (if any) is sent in the request body as `api_keys`.
 */
export const providerFormValuesToCreate = (
	values: ProviderFormValues,
): CreateAIProviderRequest => {
	const name = values.name.trim();
	const baseUrl = values.baseUrl.trim();
	const displayName = name;

	if (values.type === "bedrock") {
		const settings = buildBedrockSettings(
			undefined,
			values.model.trim(),
			values.smallFastModel.trim(),
			sanitizeCredential(values.accessKey),
			sanitizeCredential(values.accessKeySecret),
		);
		return {
			type: "anthropic",
			name,
			display_name: displayName,
			base_url: baseUrl,
			enabled: values.enabled,
			settings: settings as AIProviderSettings,
		};
	}

	const apiKey = sanitizeCredential(values.apiKey);
	return {
		type: values.type === "openai" ? "openai" : "anthropic",
		name,
		display_name: displayName,
		base_url: baseUrl,
		enabled: values.enabled,
		...(apiKey ? { api_keys: [apiKey] } : {}),
	};
};

/**
 * Build a PATCH payload for an existing provider.
 *
 * Bedrock secrets follow an "empty = keep" contract: if the user did not
 * clear the masked inputs, we omit the access-key fields from the settings
 * blob and the server leaves them unchanged. The non-secret Bedrock settings
 * (region, models) are always sent when the form holds a Bedrock provider.
 *
 * OpenAI/Anthropic API keys also follow "empty = keep": we omit `api_keys`
 * entirely when the user did not type a new value. When the user did supply
 * a new key, we send `api_keys: [{ api_key }]`. The server treats this as an
 * atomic rotation, deleting every existing key whose ID is not referenced
 * (i.e. all of them) and inserting the new plaintext.
 */
export const providerFormValuesToUpdate = (
	values: ProviderFormValues,
	existingProvider: AIProvider,
): UpdateAIProviderRequest => {
	const base: UpdateAIProviderRequest = {
		display_name: values.name.trim(),
		enabled: values.enabled,
		base_url: values.baseUrl.trim(),
	};

	if (values.type !== "bedrock") {
		// Filter out both the static `SAVED_CREDENTIAL_MASK` and the API's own
		// masked rendering of the saved key. If the user did not focus the
		// input or clear the mask, the form value is still the seed and the
		// sanitized result is `""` (no rotation).
		const savedMasked = existingProvider.api_keys[0]?.masked;
		const newApiKey = sanitizeCredential(values.apiKey, savedMasked);
		if (newApiKey === "") {
			return base;
		}
		const apiKeys: AIProviderKeyMutation[] = [{ api_key: newApiKey }];
		return { ...base, api_keys: apiKeys };
	}

	const newAccessKey = sanitizeCredential(values.accessKey);
	const newAccessKeySecret = sanitizeCredential(values.accessKeySecret);
	// Yup enforces that access key and secret are entered together before we
	// reach this point; if both survived the mask filter, the user wants to
	// rotate credentials.
	const credentialsChanged = newAccessKey !== "" && newAccessKeySecret !== "";

	// Preserve the saved region; the form doesn't surface region today.
	const savedSettings = existingProvider.settings as SettingsWire | null;
	const savedRegion = savedSettings?.region;

	const settings = buildBedrockSettings(
		savedRegion,
		values.model.trim(),
		values.smallFastModel.trim(),
		credentialsChanged ? newAccessKey : "",
		credentialsChanged ? newAccessKeySecret : "",
	);

	return { ...base, settings: settings as AIProviderSettings };
};

/**
 * Populate the form from an `AIProvider` fetched from the API.
 *
 * On the edit form the user-facing "name" field actually edits
 * `display_name` (the slug `name` is immutable server-side), so we seed
 * the form-`name` slot with `display_name` and fall back to `name` for
 * historical providers that never had a friendly label set.
 */
export const aiProviderToFormValues = (
	provider: AIProvider,
): Partial<ProviderFormValues> => {
	const editableName = provider.display_name || provider.name;
	if (isBedrockProvider(provider)) {
		const s = (provider.settings as SettingsWire | null) ?? {};
		return {
			type: "bedrock",
			name: editableName,
			baseUrl: provider.base_url,
			model: s.model ?? "",
			smallFastModel: s.small_fast_model ?? "",
			accessKey: "",
			accessKeySecret: "",
			enabled: provider.enabled,
		};
	}

	return {
		type: provider.type === "openai" ? "openai" : "anthropic",
		name: editableName,
		baseUrl: provider.base_url,
		apiKey: "",
		enabled: provider.enabled,
	};
};
