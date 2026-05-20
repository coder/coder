import type {
	AIProvider,
	AIProviderBedrockSettings,
	AIProviderKeyMutation,
	AIProviderSettings,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";
import {
	type ProviderFormValues,
	parseBedrockRegionFromBaseUrl,
	SAVED_CREDENTIAL_MASK,
} from "./ProviderForm";

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
 *
 * `display_name` is optional server-side: when the form leaves it blank we
 * omit it, the server stores NULL, and the UI falls back to `name` via the
 * standard `display_name || name` pattern.
 */
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

	const apiKey = sanitizeCredential(values.apiKey);
	// Bedrock branched out above and Yup blocks the empty default, but the
	// form-values union still allows `""`; narrow here so TS keeps us honest.
	if (values.type === "") {
		throw new Error("provider type is required");
	}
	// The codersdk validator only accepts `openai` and `anthropic` as wire
	// types today; the OpenAI-compatible UI choices (azure, google,
	// openai-compat, openrouter, vercel) collapse to `openai` on the wire and
	// surface only as dropdown presets that drive name/baseUrl/icon defaults.
	// `anthropic` is the only non-bedrock UI type with its own wire identity.
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

/**
 * Build a PATCH payload for an existing provider.
 *
 * Bedrock secrets follow an "empty = keep" contract: if the user did not
 * clear the masked inputs, we omit the access-key fields from the settings
 * blob and the server leaves them unchanged. The non-secret Bedrock settings
 * (region, models) are always sent when the form holds a Bedrock provider.
 *
 * OpenAI/Anthropic API keys ship as a declarative list: the supplied
 * `api_keys` describes the post-patch state of the key set, the server
 * reconciles it against what's stored, and any saved key whose id is absent
 * from the request is deleted. The form holds a single credential input, so
 * we send either a retain-all mutation (one `{ id }` per saved key) when the
 * user did not type a fresh value, or a single `{ api_key }` when they did.
 * The latter implicitly deletes every existing key (their ids aren't in the
 * list) and inserts the new plaintext as a fresh row.
 */
export const providerFormValuesToUpdate = (
	values: ProviderFormValues,
	existingProvider: AIProvider,
): UpdateAIProviderRequest => {
	const base: UpdateAIProviderRequest = {
		display_name: values.displayName.trim(),
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
		// Declarative wire shape: the server reconciles api_keys against the
		// supplied list. `{ id }` retains the saved row, `{ api_key }` inserts
		// a new plaintext, and any saved key whose id is absent is deleted.
		// The backend rejects sending both fields on the same entry today, so a
		// rotation goes out as the new plaintext alone: the saved key's id is
		// omitted from the list (triggering its deletion) and the plaintext is
		// inserted as a fresh row.
		const apiKeys: AIProviderKeyMutation[] =
			newApiKey === ""
				? existingProvider.api_keys.map((k) => ({ id: k.id }))
				: [{ api_key: newApiKey }];
		return { ...base, api_keys: apiKeys };
	}

	const newAccessKey = sanitizeCredential(values.accessKey);
	const newAccessKeySecret = sanitizeCredential(values.accessKeySecret);
	// Yup enforces that access key and secret are entered together before we
	// reach this point; if both survived the mask filter, the user wants to
	// rotate credentials.
	const credentialsChanged = newAccessKey !== "" && newAccessKeySecret !== "";

	// Region is derived from the canonical AWS Bedrock URL. The form schema
	// blocks non-canonical endpoints before we get here, so any saved value
	// of `undefined` is the server-validation path that the helper itself
	// must not paper over.
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

/**
 * Populate the form from an `AIProvider` fetched from the API.
 *
 * The slug `name` is immutable server-side and the update form keeps it
 * hidden, but we still seed it so the form values keep parity with the
 * `ProviderFormValues` type. The user-facing Display name field is seeded
 * from `display_name`, falling back to the slug for historical providers
 * that never had a friendly label set; once the user saves, the empty
 * `display_name` is replaced with whatever they confirm in the input.
 */
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

	// Wire `type` rolls up to one of `openai`/`anthropic` per the
	// codersdk validator; the form mirrors that and the dropdown's richer
	// OpenAI-compatible labels apply only on create.
	return {
		type: provider.type === "anthropic" ? "anthropic" : "openai",
		name: provider.name,
		displayName,
		baseUrl: provider.base_url,
		apiKey: "",
		enabled: provider.enabled,
	};
};
