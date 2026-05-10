import { describe, expect, it } from "vitest";
import { MaxChatFileSizeBytes } from "#/api/typesGenerated";
import {
	formatMiB,
	imageBudgetForProvider,
	imageNeedsResize,
	providerBudgetError,
} from "./imageBudget";

const ANTHROPIC_BUDGET = 5 * 1024 * 1024 - 16 * 1024;
const DEFAULT_BUDGET = MaxChatFileSizeBytes - 16 * 1024;

describe("imageBudgetForProvider", () => {
	it("returns the Anthropic budget for direct Anthropic", () => {
		expect(imageBudgetForProvider("anthropic")).toBe(ANTHROPIC_BUDGET);
	});

	it("returns the Anthropic budget for Bedrock (Anthropic-compatible)", () => {
		expect(imageBudgetForProvider("bedrock")).toBe(ANTHROPIC_BUDGET);
	});

	it("returns the default budget for OpenAI", () => {
		expect(imageBudgetForProvider("openai")).toBe(DEFAULT_BUDGET);
	});

	it("returns the default budget for unknown providers", () => {
		expect(imageBudgetForProvider("brand-new-provider")).toBe(DEFAULT_BUDGET);
	});

	it("returns the default budget when provider is undefined", () => {
		expect(imageBudgetForProvider(undefined)).toBe(DEFAULT_BUDGET);
	});

	// Mirrors server-side chatprovider.NormalizeProvider so a
	// caller passing a case/whitespace variant gets the strict
	// budget instead of silently falling through to the default.
	it.each([
		"Anthropic",
		"ANTHROPIC",
		" anthropic ",
		"\tanthropic\n",
		"AnThRoPiC",
	])("normalizes case/whitespace before matching strict providers (%s)", (input) => {
		expect(imageBudgetForProvider(input)).toBe(ANTHROPIC_BUDGET);
	});

	it("normalizes Bedrock variants too", () => {
		expect(imageBudgetForProvider("Bedrock")).toBe(ANTHROPIC_BUDGET);
		expect(imageBudgetForProvider(" BEDROCK ")).toBe(ANTHROPIC_BUDGET);
	});
});

describe("imageNeedsResize", () => {
	const oversize = (type: string, bytes: number): File => {
		const f = new File([new Uint8Array(8)], `f.${type.split("/")[1]}`, {
			type,
		});
		Object.defineProperty(f, "size", { value: bytes });
		return f;
	};

	it("returns true for an over-budget image", () => {
		expect(
			imageNeedsResize(oversize("image/png", 6 * 1024 * 1024), 5 * 1024 * 1024),
		).toBe(true);
	});

	it("returns false for an under-budget image", () => {
		expect(
			imageNeedsResize(oversize("image/png", 1 * 1024 * 1024), 5 * 1024 * 1024),
		).toBe(false);
	});

	it("returns false for non-image files even when oversized", () => {
		expect(
			imageNeedsResize(
				oversize("text/plain", 6 * 1024 * 1024),
				5 * 1024 * 1024,
			),
		).toBe(false);
	});
});

describe("formatMiB", () => {
	it("renders one decimal place", () => {
		expect(formatMiB(5 * 1024 * 1024)).toBe("5.0");
		expect(formatMiB(5 * 1024 * 1024 + 512 * 1024)).toBe("5.5");
		expect(formatMiB(0)).toBe("0.0");
	});
});

describe("providerBudgetError", () => {
	it("uses the provider's display label and MiB units", () => {
		const message = providerBudgetError(
			"anthropic",
			6 * 1024 * 1024,
			ANTHROPIC_BUDGET,
		);
		expect(message).toMatch(/Anthropic/);
		expect(message).toMatch(/6\.0 MiB/);
		expect(message).toMatch(/5\.0 MiB/);
	});

	it("falls back to a generic label when provider is undefined", () => {
		const message = providerBudgetError(
			undefined,
			6 * 1024 * 1024,
			ANTHROPIC_BUDGET,
		);
		expect(message).toMatch(/this provider/);
	});
});
