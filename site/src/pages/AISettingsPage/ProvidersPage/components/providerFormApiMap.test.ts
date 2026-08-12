import { describe, expect, it } from "vitest";
import type { AIProvider } from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderCopilot,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import {
	type ProviderFormValues,
	parseBedrockRegionFromBaseUrl,
	SAVED_CREDENTIAL_MASK,
} from "./ProviderForm";
import {
	aiProviderToFormValues,
	bedrockExternalId,
	getProviderDisplayType,
	hasBedrockStoredCredentials,
	isBedrockProvider,
	providerFormValuesToCreate,
	providerFormValuesToUpdate,
} from "./providerFormApiMap";

const baseOpenAIFormValues: ProviderFormValues = {
	type: "openai",
	name: "primary-openai",
	displayName: "Primary OpenAI",
	icon: "",
	baseUrl: "https://api.openai.com",
	protocol: "invoke-model",
	model: "",
	smallFastModel: "",
	accessKey: "",
	accessKeySecret: "",
	roleArn: "",
	apiKey: "sk-test",
	enabled: true,
};

const baseBedrockFormValues: ProviderFormValues = {
	type: "bedrock",
	name: "primary-bedrock",
	displayName: "Primary Bedrock",
	icon: "",
	baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
	protocol: "invoke-model",
	model: "anthropic.claude-sonnet-4-5",
	smallFastModel: "anthropic.claude-haiku-4-5",
	accessKey: "AKIA-test",
	accessKeySecret: "secret",
	roleArn: "",
	apiKey: "",
	enabled: true,
};

const baseCopilotFormValues: ProviderFormValues = {
	type: "copilot",
	name: "copilot",
	displayName: "GitHub Copilot",
	icon: "",
	baseUrl: "https://api.business.githubcopilot.com",
	protocol: "invoke-model",
	model: "",
	smallFastModel: "",
	accessKey: "",
	accessKeySecret: "",
	roleArn: "",
	apiKey: "",
	enabled: true,
};

// Cast a plain object to AIProvider's discriminated `settings` shape;
// the generated TS interface is empty and the wire form carries the
// discriminator keys flattened in alongside the variant fields.
const settings = (raw: Record<string, unknown>): AIProvider["settings"] =>
	raw as unknown as AIProvider["settings"];

describe("parseBedrockRegionFromBaseUrl", () => {
	it("extracts the region from a canonical AWS Bedrock URL", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"https://bedrock-runtime.us-east-1.amazonaws.com",
			),
		).toBe("us-east-1");
	});

	it("accepts a trailing slash", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"https://bedrock-runtime.us-west-2.amazonaws.com/",
			),
		).toBe("us-west-2");
	});

	it("extracts the region from a mantle URL", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"https://bedrock-mantle.eu-west-1.api.aws/anthropic",
			),
		).toBe("eu-west-1");
	});

	it("returns undefined for a mantle URL missing the /anthropic suffix", () => {
		// The /anthropic suffix is required, so a bare mantle host is not a
		// valid endpoint and yields no region.
		expect(
			parseBedrockRegionFromBaseUrl("https://bedrock-mantle.us-east-1.api.aws"),
		).toBeUndefined();
	});

	it("lowercases the region", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"https://bedrock-runtime.US-EAST-1.amazonaws.com",
			),
		).toBe("us-east-1");
	});

	it("trims surrounding whitespace before matching", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"  https://bedrock-runtime.us-east-1.amazonaws.com  ",
			),
		).toBe("us-east-1");
	});

	it("returns undefined for a non-AWS URL", () => {
		expect(
			parseBedrockRegionFromBaseUrl("https://bedrock.internal.example.com"),
		).toBeUndefined();
	});

	it("returns undefined for an empty string", () => {
		expect(parseBedrockRegionFromBaseUrl("")).toBeUndefined();
	});

	it("returns undefined for an http (non-https) URL", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"http://bedrock-runtime.us-east-1.amazonaws.com",
			),
		).toBeUndefined();
	});

	it("returns undefined for a URL with a path", () => {
		expect(
			parseBedrockRegionFromBaseUrl(
				"https://bedrock-runtime.us-east-1.amazonaws.com/v1/something",
			),
		).toBeUndefined();
	});

	it("returns undefined for the China partition (different TLD)", () => {
		// AWS China uses *.amazonaws.com.cn, which the canonical regex does
		// not match by design; cn-* users get the explicit Region input.
		expect(
			parseBedrockRegionFromBaseUrl(
				"https://bedrock-runtime.cn-north-1.amazonaws.com.cn",
			),
		).toBeUndefined();
	});
});

describe("isBedrockProvider", () => {
	it("recognises a legacy bedrock provider stored as type=anthropic", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			type: "anthropic",
		};
		expect(isBedrockProvider(provider)).toBe(true);
	});

	it("recognises a provider with explicit bedrock type", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			type: "bedrock",
		};
		expect(isBedrockProvider(provider)).toBe(true);
	});

	it("rejects an OpenAI provider", () => {
		expect(isBedrockProvider(MockAIProviderOpenAI)).toBe(false);
	});

	it("rejects an anthropic provider whose settings are null", () => {
		// MockAIProviderAnthropic carries `settings: null`, which the Go
		// server emits when there is no type-specific config. The helper
		// must null-check before reading `_type`.
		expect(isBedrockProvider(MockAIProviderAnthropic)).toBe(false);
	});

	it("rejects an anthropic provider whose settings carry a different discriminator", () => {
		const provider: AIProvider = {
			...MockAIProviderAnthropic,
			settings: settings({ _type: "copilot", _version: 1 }),
		};
		expect(isBedrockProvider(provider)).toBe(false);
	});
});

describe("hasBedrockStoredCredentials", () => {
	it("is true whenever the provider is Bedrock", () => {
		// Bedrock secrets are write-only, so we cannot inspect their
		// presence; the helper assumes any persisted Bedrock config
		// implies credentials are on file.
		expect(hasBedrockStoredCredentials(MockAIProviderBedrock)).toBe(true);
	});

	it("is false for non-Bedrock providers", () => {
		expect(hasBedrockStoredCredentials(MockAIProviderOpenAI)).toBe(false);
		expect(hasBedrockStoredCredentials(MockAIProviderAnthropic)).toBe(false);
	});
});

describe("bedrockExternalId", () => {
	it("returns the external ID from a role-based Bedrock provider", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: settings({
				_type: "bedrock",
				role_arn: "arn:aws:iam::123456789012:role/BedrockRole",
				external_id: "7QF3ZK2MLP4RS6TUVWXY2ABCDE",
			}),
		};
		expect(bedrockExternalId(provider)).toBe("7QF3ZK2MLP4RS6TUVWXY2ABCDE");
	});

	it("returns undefined when the provider has no external ID", () => {
		expect(bedrockExternalId(MockAIProviderBedrock)).toBeUndefined();
	});

	it("returns undefined when the external ID is an empty string", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: settings({ _type: "bedrock", external_id: "" }),
		};
		expect(bedrockExternalId(provider)).toBeUndefined();
	});

	it("returns undefined for a non-Bedrock provider", () => {
		expect(bedrockExternalId(MockAIProviderOpenAI)).toBeUndefined();
	});
});

describe("getProviderDisplayType", () => {
	it("returns bedrock for a Bedrock provider", () => {
		expect(getProviderDisplayType(MockAIProviderBedrock)).toBe("bedrock");
	});

	it("returns anthropic for a non-Bedrock Anthropic provider", () => {
		expect(getProviderDisplayType(MockAIProviderAnthropic)).toBe("anthropic");
	});

	it("returns openai for the canonical OpenAI host", () => {
		expect(getProviderDisplayType(MockAIProviderOpenAI)).toBe("openai");
	});

	it("returns copilot for a Copilot provider", () => {
		expect(getProviderDisplayType(MockAIProviderCopilot)).toBe("copilot");
	});

	it.each([
		["azure", "https://my-resource.openai.azure.com/openai/v1"],
		["azure", "https://YOUR-RESOURCE.openai.azure.com/openai/v1"],
		["google", "https://generativelanguage.googleapis.com/v1beta/openai/"],
		["openrouter", "https://openrouter.ai/api/v1"],
		["vercel", "https://ai-gateway.vercel.sh/v1"],
	])("recovers the %s preset from a canonical base_url", (expected, baseUrl) => {
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			base_url: baseUrl,
		};
		expect(getProviderDisplayType(provider)).toBe(expected);
	});

	it("preserves an explicit provider type over host detection", () => {
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			type: "openai-compat",
			base_url: "https://openrouter.ai/api/v1",
		};
		expect(getProviderDisplayType(provider)).toBe("openai-compat");
	});

	it("falls back to the wire type for an unrecognized base_url", () => {
		// Internal proxies and custom OpenAI-compatible endpoints keep the
		// OpenAI glyph rather than dropping to a question mark.
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			base_url: "https://llm-proxy.internal.example.com/v1",
		};
		expect(getProviderDisplayType(provider)).toBe("openai");
	});

	it("falls back to the wire type when base_url is not a parseable URL", () => {
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			base_url: "not a url",
		};
		expect(getProviderDisplayType(provider)).toBe("openai");
	});
});

describe("providerFormValuesToCreate", () => {
	describe("OpenAI/Anthropic", () => {
		it("sends a plaintext API key in the api_keys list", () => {
			const req = providerFormValuesToCreate(baseOpenAIFormValues);
			expect(req.type).toBe("openai");
			expect(req.api_keys).toEqual(["sk-test"]);
		});

		it("omits api_keys when the user did not type a key", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				apiKey: "",
			});
			expect(req.api_keys).toBeUndefined();
		});

		it("omits api_keys when the value is only whitespace", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				apiKey: "   ",
			});
			expect(req.api_keys).toBeUndefined();
		});

		it("does not round-trip the saved-credential mask back to the API", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				apiKey: SAVED_CREDENTIAL_MASK,
			});
			expect(req.api_keys).toBeUndefined();
		});

		it("omits display_name when blank so the server stores NULL", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				displayName: "",
			});
			expect(req.display_name).toBeUndefined();
		});

		it("trims and sends icon when provided", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				icon: "  https://example.com/openai.svg  ",
			});
			expect(req.icon).toBe("https://example.com/openai.svg");
		});

		it("omits icon when blank", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				icon: "   ",
			});
			expect(req.icon).toBeUndefined();
		});

		it("trims whitespace from name and baseUrl", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				name: "  primary-openai  ",
				baseUrl: "  https://api.openai.com  ",
			});
			expect(req.name).toBe("primary-openai");
			expect(req.base_url).toBe("https://api.openai.com");
		});

		it("preserves the Anthropic provider type", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				type: "anthropic",
				baseUrl: "https://api.anthropic.com",
			});
			expect(req.type).toBe("anthropic");
			expect(req.base_url).toBe("https://api.anthropic.com");
			expect(req.api_keys).toEqual(["sk-test"]);
		});

		it.each([
			["azure", "https://YOUR-RESOURCE.openai.azure.com/openai/v1"],
			["google", "https://generativelanguage.googleapis.com/v1beta/openai/"],
			["openai-compat", "https://compat.example.com/v1"],
			["openrouter", "https://openrouter.ai/api/v1"],
			["vercel", "https://ai-gateway.vercel.sh/v1"],
		] as const)("preserves the %s provider type", (type, baseUrl) => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				type,
				baseUrl,
			});
			expect(req.type).toBe(type);
			expect(req.base_url).toBe(baseUrl);
			expect(req.api_keys).toEqual(["sk-test"]);
		});

		it("rejects an empty type", () => {
			// `type: ""` is blocked by the Yup schema; the helper still has
			// to refuse to send a malformed payload if a caller bypasses it.
			expect(() =>
				providerFormValuesToCreate({ ...baseOpenAIFormValues, type: "" }),
			).toThrowError(/provider type is required/);
		});
	});

	describe("Bedrock", () => {
		it('maps Bedrock to a wire `type:"bedrock"`', () => {
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			expect(req.type).toBe("bedrock");
		});

		it("derives the region from a canonical AWS URL", () => {
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s._type).toBe("bedrock");
			expect(s.region).toBe("us-east-1");
		});

		it("emits protocol=invoke-model and includes the model fields for InvokeModel", () => {
			// The protocol is emitted explicitly, and the model fields are
			// configured on the provider.
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.protocol).toBe("invoke-model");
			expect(s.model).toBe("anthropic.claude-sonnet-4-5");
			expect(s.small_fast_model).toBe("anthropic.claude-haiku-4-5");
		});

		it("sets protocol=mantle, derives the region, and omits the model fields", () => {
			// Mantle is a passthrough: the client sends the model, so the
			// provider stores neither model field but keeps the region so the
			// backend recognises the Bedrock provider.
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				protocol: "mantle",
				baseUrl: "https://bedrock-mantle.us-east-1.api.aws/anthropic",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s._type).toBe("bedrock");
			expect(s.protocol).toBe("mantle");
			expect(s.region).toBe("us-east-1");
			expect(s.model).toBeUndefined();
			expect(s.small_fast_model).toBeUndefined();
		});

		it("omits the region when the URL is non-canonical", () => {
			// The form schema blocks non-canonical endpoints before submit; the
			// helper itself stays strict, returning an undefined region rather
			// than inventing a value.
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				baseUrl: "https://bedrock.internal.example.com",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.region).toBeUndefined();
		});

		it("includes access_key and access_key_secret when provided", () => {
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.access_key).toBe("AKIA-test");
			expect(s.access_key_secret).toBe("secret");
		});

		it("omits the access fields when the form values are blank", () => {
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				accessKey: "",
				accessKeySecret: "",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.access_key).toBeUndefined();
			expect(s.access_key_secret).toBeUndefined();
		});

		it("keeps the region so the backend recognises the Bedrock provider when access keys are omitted", () => {
			// The backend treats Region as a configuration signal
			// (codersdk.AIProviderBedrockSettings.IsConfigured), so omitting
			// the keys must not also strip the region; otherwise the request
			// would fail with "type=bedrock requires bedrock settings".
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				accessKey: "",
				accessKeySecret: "",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.region).toBe("us-east-1");
			expect(s._type).toBe("bedrock");
		});

		it("omits the access fields when only whitespace is supplied", () => {
			// Mirrors the OpenAI/Anthropic whitespace handling: callers must
			// not accidentally persist a credential whose plaintext is just
			// blanks.
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				accessKey: "   ",
				accessKeySecret: "\t",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.access_key).toBeUndefined();
			expect(s.access_key_secret).toBeUndefined();
		});

		it("ignores the OpenAI/Anthropic api key field", () => {
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				apiKey: "should-be-ignored",
			});
			expect(req.api_keys).toBeUndefined();
		});

		it("includes role_arn when a role ARN is provided", () => {
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				roleArn: "arn:aws:iam::123456789012:role/BedrockRole",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.role_arn).toBe("arn:aws:iam::123456789012:role/BedrockRole");
		});

		it("omits role_arn when the form value is blank", () => {
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.role_arn).toBeUndefined();
		});

		it("trims whitespace around the role ARN", () => {
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				roleArn: "  arn:aws:iam::123456789012:role/BedrockRole  ",
			});
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.role_arn).toBe("arn:aws:iam::123456789012:role/BedrockRole");
		});
	});

	describe("Copilot", () => {
		it("maps to a distinct wire type with no api_keys", () => {
			const req = providerFormValuesToCreate(baseCopilotFormValues);
			expect(req.type).toBe("copilot");
			expect(req.base_url).toBe("https://api.business.githubcopilot.com");
			expect(req.api_keys).toBeUndefined();
		});

		it("never sends api_keys even if the field carries a value", () => {
			const req = providerFormValuesToCreate({
				...baseCopilotFormValues,
				apiKey: "should-be-ignored",
			});
			expect(req.api_keys).toBeUndefined();
		});

		it("omits display_name when blank so the server stores NULL", () => {
			const req = providerFormValuesToCreate({
				...baseCopilotFormValues,
				displayName: "",
			});
			expect(req.display_name).toBeUndefined();
		});
	});
});

describe("providerFormValuesToUpdate", () => {
	describe("OpenAI/Anthropic", () => {
		it("sends api_keys as a single-entry rotation list when a new key is typed", () => {
			const req = providerFormValuesToUpdate(
				{ ...baseOpenAIFormValues, apiKey: "sk-new" },
				MockAIProviderOpenAI,
			);
			expect(req.api_keys).toEqual([{ api_key: "sk-new" }]);
		});

		it("retains the saved key by id when the user left the masked rendering", () => {
			// Seed the form with the saved masked rendering exactly as
			// the API returns it; the declarative payload must reference
			// the saved id so the server keeps the row.
			const req = providerFormValuesToUpdate(
				{
					...baseOpenAIFormValues,
					apiKey: MockAIProviderOpenAI.api_keys[0].masked,
				},
				MockAIProviderOpenAI,
			);
			expect(req.api_keys).toEqual([
				{ id: MockAIProviderOpenAI.api_keys[0].id },
			]);
		});

		it("retains the saved key by id when the user left SAVED_CREDENTIAL_MASK", () => {
			const req = providerFormValuesToUpdate(
				{ ...baseOpenAIFormValues, apiKey: SAVED_CREDENTIAL_MASK },
				MockAIProviderOpenAI,
			);
			expect(req.api_keys).toEqual([
				{ id: MockAIProviderOpenAI.api_keys[0].id },
			]);
		});

		it("sends an empty api_keys list when no key was saved and none was typed", () => {
			// Declarative wire shape: an empty list is the explicit "no keys"
			// state, matching the user's intent for a provider that never had
			// a credential on file.
			const req = providerFormValuesToUpdate(
				{ ...baseOpenAIFormValues, apiKey: "" },
				MockAIProviderAnthropic,
			);
			expect(req.api_keys).toEqual([]);
		});

		it("sends a trimmed icon so blank clears the stored icon", () => {
			const req = providerFormValuesToUpdate(
				{ ...baseOpenAIFormValues, icon: "   " },
				MockAIProviderOpenAI,
			);
			expect(req.icon).toBe("");
		});
	});

	describe("Bedrock", () => {
		it("derives the region from the canonical URL", () => {
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					baseUrl: "https://bedrock-runtime.us-west-2.amazonaws.com",
					accessKey: SAVED_CREDENTIAL_MASK,
					accessKeySecret: SAVED_CREDENTIAL_MASK,
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.region).toBe("us-west-2");
		});

		it("omits the region when the URL is non-canonical", () => {
			// The form schema blocks non-canonical endpoints before submit; the
			// helper itself stays strict, returning an undefined region rather
			// than inventing a value.
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					baseUrl: "https://bedrock.internal.example.com",
					accessKey: SAVED_CREDENTIAL_MASK,
					accessKeySecret: SAVED_CREDENTIAL_MASK,
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.region).toBeUndefined();
		});

		it("omits access_key/access_key_secret when the user left both masked (empty = keep)", () => {
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					accessKey: SAVED_CREDENTIAL_MASK,
					accessKeySecret: SAVED_CREDENTIAL_MASK,
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.access_key).toBeUndefined();
			expect(s.access_key_secret).toBeUndefined();
		});

		it("sends new access keys when both were typed", () => {
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					accessKey: "AKIA-rotate",
					accessKeySecret: "rotated-secret",
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.access_key).toBe("AKIA-rotate");
			expect(s.access_key_secret).toBe("rotated-secret");
		});

		it('treats a half-rotated credential pair as "do not rotate"', () => {
			// Yup blocks this at the schema layer; the helper still has
			// to refuse to send a partial rotation, lest a partial wire
			// payload corrupt the stored credential.
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					accessKey: "AKIA-rotate",
					accessKeySecret: SAVED_CREDENTIAL_MASK,
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.access_key).toBeUndefined();
			expect(s.access_key_secret).toBeUndefined();
		});

		it("sends role_arn even when the access keys are kept", () => {
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					accessKey: SAVED_CREDENTIAL_MASK,
					accessKeySecret: SAVED_CREDENTIAL_MASK,
					roleArn: "arn:aws:iam::123456789012:role/BedrockRole",
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.role_arn).toBe("arn:aws:iam::123456789012:role/BedrockRole");
		});

		it("omits role_arn when the field is cleared", () => {
			const req = providerFormValuesToUpdate(
				{
					...baseBedrockFormValues,
					roleArn: "",
				},
				MockAIProviderBedrock,
			);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s.role_arn).toBeUndefined();
		});
	});

	describe("Copilot", () => {
		it("patches only the base fields and never sends api_keys", () => {
			const req = providerFormValuesToUpdate(
				{ ...baseCopilotFormValues, apiKey: "should-be-ignored" },
				MockAIProviderCopilot,
			);
			expect(req.api_keys).toBeUndefined();
			expect(req.settings).toBeUndefined();
			expect(req.base_url).toBe("https://api.business.githubcopilot.com");
		});
	});
});

describe("aiProviderToFormValues", () => {
	it("seeds OpenAI form values from a wire provider", () => {
		const values = aiProviderToFormValues({
			...MockAIProviderOpenAI,
			icon: "https://example.com/openai.svg",
		});
		expect(values.type).toBe("openai");
		expect(values.name).toBe(MockAIProviderOpenAI.name);
		expect(values.baseUrl).toBe(MockAIProviderOpenAI.base_url);
		expect(values.icon).toBe("https://example.com/openai.svg");
		expect(values.apiKey).toBe("");
	});

	it.each([
		["azure", "https://YOUR-RESOURCE.openai.azure.com/openai/v1"],
		["google", "https://generativelanguage.googleapis.com/v1beta/openai/"],
		["openai-compat", "https://compat.example.com/v1"],
		["openrouter", "https://openrouter.ai/api/v1"],
		["vercel", "https://ai-gateway.vercel.sh/v1"],
	] as const)("seeds %s form values from the provider type", (type, baseUrl) => {
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			type,
			base_url: baseUrl,
		};
		const values = aiProviderToFormValues(provider);
		expect(values.type).toBe(type);
		expect(values.baseUrl).toBe(baseUrl);
	});

	it("uses the Google preset for a generic provider with the Google host", () => {
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			base_url: "https://generativelanguage.googleapis.com/v1beta/openai/",
		};
		const values = aiProviderToFormValues(provider);
		expect(values.type).toBe("google");
	});

	it("seeds Bedrock form values from settings", () => {
		const values = aiProviderToFormValues(MockAIProviderBedrock);
		expect(values.type).toBe("bedrock");
		expect(values.model).toBe("anthropic.claude-opus-4-7");
		expect(values.smallFastModel).toBe("anthropic.claude-haiku-4-5");
	});

	it("seeds Bedrock form values from a legacy anthropic-typed provider", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			type: "anthropic",
		};
		const values = aiProviderToFormValues(provider);
		expect(values.type).toBe("bedrock");
		expect(values.model).toBe("anthropic.claude-opus-4-7");
		expect(values.smallFastModel).toBe("anthropic.claude-haiku-4-5");
	});

	it("reads protocol=mantle back and leaves the model fields blank", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: settings({
				_type: "bedrock",
				protocol: "mantle",
				region: "us-east-1",
			}),
		};
		const values = aiProviderToFormValues(provider);
		expect(values.protocol).toBe("mantle");
		expect(values.model).toBe("");
		expect(values.smallFastModel).toBe("");
	});

	it("defaults protocol to invoke-model for a legacy provider without one", () => {
		const values = aiProviderToFormValues(MockAIProviderBedrock);
		expect(values.protocol).toBe("invoke-model");
	});

	it("resolves an empty stored protocol to invoke-model", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: settings({
				_type: "bedrock",
				protocol: "",
				region: "us-east-1",
			}),
		};
		const values = aiProviderToFormValues(provider);
		expect(values.protocol).toBe("invoke-model");
	});

	it("never round-trips Bedrock secrets back to the form", () => {
		// AccessKey and AccessKeySecret are write-only; the API strips
		// them from responses, so the form must seed them as empty.
		const values = aiProviderToFormValues(MockAIProviderBedrock);
		expect(values.accessKey).toBe("");
		expect(values.accessKeySecret).toBe("");
	});

	it("round-trips role_arn back into the form", () => {
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: settings({
				_type: "bedrock",
				role_arn: "arn:aws:iam::123456789012:role/BedrockRole",
			}),
		};
		const values = aiProviderToFormValues(provider);
		expect(values.roleArn).toBe("arn:aws:iam::123456789012:role/BedrockRole");
	});

	it("seeds an empty role ARN when the provider has none", () => {
		const values = aiProviderToFormValues(MockAIProviderBedrock);
		expect(values.roleArn).toBe("");
	});

	it("seeds Copilot form values without a credential field", () => {
		const values = aiProviderToFormValues(MockAIProviderCopilot);
		expect(values.type).toBe("copilot");
		expect(values.name).toBe(MockAIProviderCopilot.name);
		expect(values.baseUrl).toBe(MockAIProviderCopilot.base_url);
		expect(values.apiKey).toBeUndefined();
	});

	it("falls back to the slug when display_name is empty", () => {
		const provider: AIProvider = {
			...MockAIProviderOpenAI,
			display_name: "",
		};
		expect(aiProviderToFormValues(provider).displayName).toBe(provider.name);
	});

	it("handles a Bedrock provider whose settings are null", () => {
		// `isBedrockProvider` will return false, so the provider falls
		// through to the generic branch. The helper must not throw.
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: null as unknown as AIProvider["settings"],
		};
		const values = aiProviderToFormValues(provider);
		expect(values.type).toBe("bedrock");
	});
});
