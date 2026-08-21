import { describe, expect, it } from "vitest";
import { MockChatModel } from "#/testHelpers/chatModels";
import { getModelDisplayName } from "./modelDisplayName";

describe("getModelDisplayName", () => {
	it("shows the matching model display name", () => {
		expect(
			getModelDisplayName("model-1", [
				{ ...MockChatModel, display_name: "GPT-5" },
			]),
		).toBe("GPT-5");
	});

	it("shows Default model for an unset historical ID", () => {
		expect(getModelDisplayName("", [])).toBe("Default model");
		expect(
			getModelDisplayName("00000000-0000-0000-0000-000000000000", []),
		).toBe("Default model");
	});

	it("shows Unavailable model for an unknown non-empty historical ID", () => {
		expect(getModelDisplayName("foreign-model", [])).toBe("Unavailable model");
	});
});
