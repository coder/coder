import * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../components/ChatElements";
import {
	availableExternalChatRuntimes,
	externalChatRuntimes,
	filterModelOptionsForRuntime,
	isExternalChatRuntime,
} from "./chatRuntimes";

const option = (
	id: string,
	provider: TypesGen.AIProviderType,
): ModelSelectorOption => ({
	id,
	provider,
	model: id,
	displayName: id,
});

const options = [
	option("sonnet", "anthropic"),
	option("gpt", "openai"),
	option("gemini", "google"),
];

describe("isExternalChatRuntime", () => {
	it("excludes the built-in coder runtime and missing values", () => {
		expect(isExternalChatRuntime("coder")).toBe(false);
		expect(isExternalChatRuntime(undefined)).toBe(false);
		expect(isExternalChatRuntime("claude_code")).toBe(true);
		expect(isExternalChatRuntime("codex")).toBe(true);
	});
});

describe("filterModelOptionsForRuntime", () => {
	it.each(TypesGen.ChatRuntimes.filter(isExternalChatRuntime))(
		"keeps only %s provider models",
		(runtime) => {
			const filtered = filterModelOptionsForRuntime(options, runtime);
			expect(filtered).toHaveLength(1);
			expect(filtered[0]?.provider).toBe(
				externalChatRuntimes[runtime].providerType,
			);
		},
	);
});

describe("availableExternalChatRuntimes", () => {
	const availability: TypesGen.ChatRuntimeAvailability[] = [
		{ organization_id: "org-a", runtime: "codex" },
		{ organization_id: "org-a", runtime: "claude_code" },
		{ organization_id: "org-b", runtime: "codex" },
		{ organization_id: "org-a", runtime: "codex" },
	];

	it("returns each runtime once for the organization, in table order", () => {
		expect(availableExternalChatRuntimes(availability, "org-a")).toEqual([
			"claude_code",
			"codex",
		]);
		expect(availableExternalChatRuntimes(availability, "org-b")).toEqual([
			"codex",
		]);
	});

	it("returns nothing for unknown organizations or missing data", () => {
		expect(availableExternalChatRuntimes(availability, "org-c")).toEqual([]);
		expect(availableExternalChatRuntimes(undefined, "org-a")).toEqual([]);
	});
});
