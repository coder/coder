import { describe, expect, it } from "vitest";
import type { AIProvider } from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import {
	type ProviderFormValues,
	parseBedrockRegionFromBaseUrl,
	SAVED_CREDENTIAL_MASK,
} from "./ProviderForm";
import {
	aiProviderToFormValues,
	hasBedrockStoredCredentials,
	isBedrockProvider,
	providerFormValuesToCreate,
	providerFormValuesToUpdate,
} from "./providerFormApiMap";

const baseOpenAIFormValues: ProviderFormValues = {
	type: "openai",
	name: "primary-openai",
	displayName: "Primary OpenAI",
	baseUrl: "https://api.openai.com",
	model: "",
	smallFastModel: "",
	accessKey: "",
	accessKeySecret: "",
	apiKey: "sk-test",
	enabled: true,
};

const baseBedrockFormValues: ProviderFormValues = {
	type: "bedrock",
	name: "primary-bedrock",
	displayName: "Primary Bedrock",
	baseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
	model: "anthropic.claude-sonnet-4-5",
	smallFastModel: "anthropic.claude-haiku-4-5",
	accessKey: "AKIA-test",
	accessKeySecret: "secret",
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
	it("recognises a discriminated bedrock provider", () => {
		expect(isBedrockProvider(MockAIProviderBedrock)).toBe(true);
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

		it("trims whitespace from name and baseUrl", () => {
			const req = providerFormValuesToCreate({
				...baseOpenAIFormValues,
				name: "  primary-openai  ",
				baseUrl: "  https://api.openai.com  ",
			});
			expect(req.name).toBe("primary-openai");
			expect(req.base_url).toBe("https://api.openai.com");
		});

		it.each([
			["azure", "https://YOUR-RESOURCE.openai.azure.com/openai/v1"],
			["google", "https://generativelanguage.googleapis.com/v1beta/openai/"],
			["openai-compat", "https://compat.example.com/v1"],
			["openrouter", "https://openrouter.ai/api/v1"],
			["vercel", "https://ai-gateway.vercel.sh/v1"],
		] as const)("passes the %s type through on the wire", (type, baseUrl) => {
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
		it('maps Bedrock to a wire `type:"anthropic"`', () => {
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			expect(req.type).toBe("anthropic");
		});

		it("derives the region from a canonical AWS URL", () => {
			const req = providerFormValuesToCreate(baseBedrockFormValues);
			const s = req.settings as unknown as Record<string, unknown>;
			expect(s._type).toBe("bedrock");
			expect(s.region).toBe("us-east-1");
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

		it("ignores the OpenAI/Anthropic api key field", () => {
			const req = providerFormValuesToCreate({
				...baseBedrockFormValues,
				apiKey: "should-be-ignored",
			});
			expect(req.api_keys).toBeUndefined();
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
	});
});

describe("aiProviderToFormValues", () => {
	it("seeds OpenAI form values from a wire provider", () => {
		const values = aiProviderToFormValues(MockAIProviderOpenAI);
		expect(values.type).toBe("openai");
		expect(values.name).toBe(MockAIProviderOpenAI.name);
		expect(values.baseUrl).toBe(MockAIProviderOpenAI.base_url);
		expect(values.apiKey).toBe("");
	});

	it("seeds Bedrock form values from settings", () => {
		const values = aiProviderToFormValues(MockAIProviderBedrock);
		expect(values.type).toBe("bedrock");
		expect(values.model).toBe("anthropic.claude-opus-4-7");
		expect(values.smallFastModel).toBe("anthropic.claude-haiku-4-5");
	});

	it("never round-trips Bedrock secrets back to the form", () => {
		// AccessKey and AccessKeySecret are write-only; the API strips
		// them from responses, so the form must seed them as empty.
		const values = aiProviderToFormValues(MockAIProviderBedrock);
		expect(values.accessKey).toBe("");
		expect(values.accessKeySecret).toBe("");
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
		// through to the anthropic branch. The helper must not throw.
		const provider: AIProvider = {
			...MockAIProviderBedrock,
			settings: null as unknown as AIProvider["settings"],
		};
		const values = aiProviderToFormValues(provider);
		expect(values.type).toBe("anthropic");
	});
});
