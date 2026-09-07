import type {
	AIProvider,
	AIProviderBedrockProtocol,
	AIProviderBedrockSettings,
	AIProviderClaudePlatformAWSAuthMode,
	AIProviderClaudePlatformAWSSettings,
	AIProviderKeyMutation,
	AIProviderSettings,
	AIProviderType,
	CreateAIProviderRequest,
	UpdateAIProviderRequest,
} from "#/api/typesGenerated";
import {
	AIProviderClaudePlatformAWSSettingsVersion,
	AIProviderSettingsTypeClaudePlatformAWS,
} from "#/api/typesGenerated";
import { CLAUDE_PLATFORM_DISPLAY_TYPE } from "./claudePlatform";
import {
	type ProviderFormValues,
	parseBedrockRegionFromBaseUrl,
	SAVED_CREDENTIAL_MASK,
} from "./ProviderForm";
import { getProviderIcon } from "./ProviderIcon";

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

type ProviderDisplayType = AIProviderType | typeof CLAUDE_PLATFORM_DISPLAY_TYPE;

type BedrockSettingsWire = AIProviderBedrockSettings & {
	_type: typeof BEDROCK_SETTINGS_TYPE;
	_version: typeof BEDROCK_SETTINGS_VERSION;
};

type ClaudePlatformSettingsWire = AIProviderClaudePlatformAWSSettings & {
	_type: typeof AIProviderSettingsTypeClaudePlatformAWS;
	_version: typeof AIProviderClaudePlatformAWSSettingsVersion;
};

type SettingsWire = AIProviderSettings &
	Partial<AIProviderBedrockSettings> &
	Partial<AIProviderClaudePlatformAWSSettings> & {
		_type?: string;
		_version?: number;
	};

// Bedrock providers are identified by the settings discriminator. The
// generated type marks settings as non-null, but Go serializes zero settings
// as JSON `null`.
export const isBedrockProvider = (provider: AIProvider): boolean => {
	if (provider.type !== "anthropic" && provider.type !== "bedrock") {
		return false;
	}
	const s = provider.settings as SettingsWire | null;
	return s !== null && s._type === BEDROCK_SETTINGS_TYPE;
};

// Claude Platform is only valid on `anthropic`; the server rejects the
// settings on any other type.
export const isClaudePlatformProvider = (provider: AIProvider): boolean => {
	if (provider.type !== "anthropic") {
		return false;
	}
	const s = provider.settings as SettingsWire | null;
	return s !== null && s._type === AIProviderSettingsTypeClaudePlatformAWS;
};

// The stored authentication mode, or undefined for providers that are not
// Claude Platform. An unset mode is invalid server-side, so it is reported as
// missing rather than defaulted.
export const claudePlatformAuthMode = (
	provider: AIProvider,
): AIProviderClaudePlatformAWSAuthMode | undefined => {
	if (!isClaudePlatformProvider(provider)) {
		return undefined;
	}
	const s = provider.settings as SettingsWire | null;
	return s?.auth_mode || undefined;
};

// Server-generated STS external ID; read-only. Shared by the two AWS-signed
// authentication methods, which both assume a role the same way.
export const awsExternalId = (provider: AIProvider): string | undefined => {
	if (!isBedrockProvider(provider) && !isClaudePlatformProvider(provider)) {
		return undefined;
	}
	const s = provider.settings as SettingsWire | null;
	return s?.external_id || undefined;
};

// Whether to seed the AWS credential inputs with a saved-value mask. The
// secrets are write-only, so their presence cannot be observed and an
// AWS-signed provider is assumed to have them on file. A provider relying on
// the ambient credential chain therefore also shows the mask, which is
// harmless: an unedited masked field submits as "keep unchanged".
export const hasAwsStoredCredentials = (provider: AIProvider): boolean =>
	isBedrockProvider(provider) || claudePlatformAuthMode(provider) === "iam";

const parseProviderHost = (url: string): string => {
	try {
		return new URL(url).host.toLowerCase();
	} catch {
		return "";
	}
};

// Preset types can be recovered from a saved generic OpenAI provider's
// base_url. Matches the bare domain or any subdomain. Azure assigns
// per-resource subdomains such as my-resource.openai.azure.com.
const displayTypeHosts: ReadonlyArray<[string, AIProviderType]> = [
	["openai.azure.com", "azure"],
	["generativelanguage.googleapis.com", "google"],
	["openrouter.ai", "openrouter"],
	["ai-gateway.vercel.sh", "vercel"],
];

const matchesHost = (host: string, suffix: string): boolean =>
	host === suffix || host.endsWith(`.${suffix}`);

// Determines which UI provider type to show for a saved provider. Bedrock and
// Claude Platform are detected via settings. Explicit stored types are
// authoritative. Generic `openai` rows fall back to host inference from known
// preset endpoints; unrecognized hosts stay as `openai`.
export const getProviderDisplayType = (
	provider: AIProvider,
): ProviderDisplayType => {
	if (isBedrockProvider(provider)) {
		return "bedrock";
	}
	if (isClaudePlatformProvider(provider)) {
		return CLAUDE_PLATFORM_DISPLAY_TYPE;
	}
	if (provider.type !== "openai") {
		return provider.type;
	}
	const host = parseProviderHost(provider.base_url ?? "");
	const match = displayTypeHosts.find(([h]) => matchesHost(host, h));
	return match?.[1] ?? provider.type;
};

const buildBedrockSettings = (
	protocol: AIProviderBedrockProtocol,
	region: string | undefined,
	model: string,
	smallFastModel: string,
	accessKey: string,
	accessKeySecret: string,
	roleArn: string,
): BedrockSettingsWire => {
	// Mantle is a passthrough protocol: the client sends the model, so the
	// provider omits the model fields. The protocol is always emitted so the
	// stored settings state it explicitly instead of relying on an absent
	// value resolving to InvokeModel server-side.
	const isMantle = protocol === "mantle";
	return {
		_type: BEDROCK_SETTINGS_TYPE,
		_version: BEDROCK_SETTINGS_VERSION,
		...(region ? { region } : {}),
		protocol,
		...(isMantle ? {} : { model, small_fast_model: smallFastModel }),
		...(accessKey ? { access_key: accessKey } : {}),
		...(accessKeySecret ? { access_key_secret: accessKeySecret } : {}),
		...(roleArn ? { role_arn: roleArn } : {}),
	};
};

// Claude Platform for AWS routing and authentication. Region is always sent,
// even when the endpoint is overridden for a proxy, because the SigV4
// signature is region-scoped. In api_key mode the workspace key lives in
// `api_keys`, so no credential is emitted here.
//
// `clearUnusedCredentials` writes explicit empty strings for the write-only
// AWS secrets. The server keeps omitted secrets, so moving an existing
// provider to api_key mode has to state the clear or stale signing keys stay
// on the row.
const buildClaudePlatformSettings = (
	authMode: AIProviderClaudePlatformAWSAuthMode,
	region: string,
	workspaceId: string,
	accessKey: string,
	accessKeySecret: string,
	roleArn: string,
	clearUnusedCredentials = false,
): ClaudePlatformSettingsWire => {
	const isIam = authMode === "iam";
	const clearAws = !isIam && clearUnusedCredentials;
	return {
		_type: AIProviderSettingsTypeClaudePlatformAWS,
		_version: AIProviderClaudePlatformAWSSettingsVersion,
		auth_mode: authMode,
		region,
		workspace_id: workspaceId,
		...(clearAws ? { access_key: "", access_key_secret: "" } : {}),
		...(isIam && accessKey ? { access_key: accessKey } : {}),
		...(isIam && accessKeySecret ? { access_key_secret: accessKeySecret } : {}),
		...(isIam && roleArn ? { role_arn: roleArn } : {}),
	};
};

const isClaudePlatformValues = (values: ProviderFormValues): boolean =>
	values.type === "anthropic" && values.authMethod === "claude_platform_aws";

const claudePlatformSettingsFromValues = (
	values: ProviderFormValues,
	accessKey: string,
	accessKeySecret: string,
	clearUnusedCredentials = false,
): ClaudePlatformSettingsWire =>
	buildClaudePlatformSettings(
		values.claudePlatformAuthMode,
		values.claudePlatformRegion.trim(),
		values.claudePlatformWorkspaceId.trim(),
		accessKey,
		accessKeySecret,
		values.roleArn.trim(),
		clearUnusedCredentials,
	);

// Bedrock credentials live in `settings`; openai/anthropic keys go in
// `api_keys`. `display_name` is omitted when blank so the server stores
// NULL and the UI falls back to `name`.
export const providerFormValuesToCreate = (
	values: ProviderFormValues,
): CreateAIProviderRequest => {
	const displayName = values.displayName.trim();
	const icon = values.icon.trim();
	const base: Omit<CreateAIProviderRequest, "type"> = {
		name: values.name.trim(),
		...(displayName ? { display_name: displayName } : {}),
		...(icon ? { icon } : {}),
		base_url: values.baseUrl.trim(),
		enabled: values.enabled,
	};

	if (values.type === "bedrock") {
		const region = parseBedrockRegionFromBaseUrl(base.base_url);
		const settings = buildBedrockSettings(
			values.protocol,
			region,
			values.model.trim(),
			values.smallFastModel.trim(),
			sanitizeCredential(values.accessKey),
			sanitizeCredential(values.accessKeySecret),
			values.roleArn.trim(),
		);
		return {
			type: "bedrock",
			...base,
			settings: settings as AIProviderSettings,
		};
	}

	if (isClaudePlatformValues(values)) {
		const settings = claudePlatformSettingsFromValues(
			values,
			sanitizeCredential(values.accessKey),
			sanitizeCredential(values.accessKeySecret),
		);
		// The server requires at least one key in api_key mode and rejects keys
		// in iam mode, where requests are signed instead.
		const workspaceKey =
			values.claudePlatformAuthMode === "api_key"
				? sanitizeCredential(values.apiKey)
				: "";
		return {
			type: "anthropic",
			...base,
			...(workspaceKey ? { api_keys: [workspaceKey] } : {}),
			settings: settings as AIProviderSettings,
		};
	}

	if (values.type === "copilot") {
		return { type: "copilot", ...base };
	}

	const apiKey = sanitizeCredential(values.apiKey);
	// `""` is unreachable here (Yup blocks it, Bedrock and Copilot branched
	// out), but the union still includes it; narrow so TS stays honest.
	if (values.type === "") {
		throw new Error("provider type is required");
	}
	return {
		type: values.type,
		...base,
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
		icon: values.icon.trim(),
		enabled: values.enabled,
		base_url: values.baseUrl.trim(),
	};

	if (values.type === "copilot") {
		return base;
	}

	if (isClaudePlatformValues(values)) {
		const newAccessKey = sanitizeCredential(values.accessKey);
		const newAccessKeySecret = sanitizeCredential(values.accessKeySecret);
		// Yup enforces "both keys together"; if both survived the mask filter,
		// the user is rotating credentials.
		const credentialsChanged = newAccessKey !== "" && newAccessKeySecret !== "";
		const settings = claudePlatformSettingsFromValues(
			values,
			credentialsChanged ? newAccessKey : "",
			credentialsChanged ? newAccessKeySecret : "",
			true,
		);

		// The auth mode and the key set have to agree after the patch, so the
		// key list is always sent: iam mode clears the keys, and api_key mode
		// keeps or rotates the workspace key. Sending only the settings would
		// make switching modes a guaranteed 400.
		const savedMasked = existingProvider.api_keys[0]?.masked;
		const newWorkspaceKey = sanitizeCredential(values.apiKey, savedMasked);
		let apiKeys: AIProviderKeyMutation[] = [];
		if (values.claudePlatformAuthMode === "api_key") {
			apiKeys =
				newWorkspaceKey === ""
					? existingProvider.api_keys.map((k) => ({ id: k.id }))
					: [{ api_key: newWorkspaceKey }];
		}
		return {
			...base,
			api_keys: apiKeys,
			settings: settings as AIProviderSettings,
		};
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
		values.protocol,
		region,
		values.model.trim(),
		values.smallFastModel.trim(),
		credentialsChanged ? newAccessKey : "",
		credentialsChanged ? newAccessKeySecret : "",
		values.roleArn.trim(),
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
		// An empty or missing protocol resolves to InvokeModel (legacy rows),
		// mirroring the backend. Any other stored value passes through unchanged
		// rather than being collapsed to InvokeModel.
		const protocol: AIProviderBedrockProtocol = s.protocol || "invoke-model";
		return {
			type: "bedrock",
			name: provider.name,
			displayName,
			icon: provider.icon || (getProviderIcon("bedrock") ?? ""),
			baseUrl: provider.base_url,
			protocol,
			// Mantle providers store no model fields, so these resolve to "".
			model: s.model ?? "",
			smallFastModel: s.small_fast_model ?? "",
			accessKey: "",
			accessKeySecret: "",
			roleArn: s.role_arn ?? "",
			enabled: provider.enabled,
		};
	}

	if (provider.type === "copilot") {
		return {
			type: "copilot",
			name: provider.name,
			displayName,
			icon: provider.icon || (getProviderIcon("copilot") ?? ""),
			baseUrl: provider.base_url,
			enabled: provider.enabled,
		};
	}

	if (isClaudePlatformProvider(provider)) {
		const s = (provider.settings as SettingsWire | null) ?? {};
		// The server requires auth_mode, so the fallback only guards a
		// hand-edited or malformed settings blob.
		const authMode = s.auth_mode ?? "iam";
		return {
			type: "anthropic",
			authMethod: "claude_platform_aws",
			name: provider.name,
			displayName,
			icon:
				provider.icon || (getProviderIcon(CLAUDE_PLATFORM_DISPLAY_TYPE) ?? ""),
			baseUrl: provider.base_url,
			claudePlatformAuthMode: authMode,
			claudePlatformRegion: s.region ?? "",
			claudePlatformWorkspaceId: s.workspace_id ?? "",
			accessKey: "",
			accessKeySecret: "",
			roleArn: s.role_arn ?? "",
			apiKey: "",
			enabled: provider.enabled,
		};
	}

	const displayType = getProviderDisplayType(provider);
	// The synthetic Claude Platform display type maps back to the provider type
	// it is an authentication method on.
	const type: AIProviderType =
		displayType === CLAUDE_PLATFORM_DISPLAY_TYPE ? "anthropic" : displayType;
	return {
		type,
		name: provider.name,
		displayName,
		icon: provider.icon || (getProviderIcon(displayType) ?? ""),
		baseUrl: provider.base_url,
		apiKey: "",
		enabled: provider.enabled,
	};
};
