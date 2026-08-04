import { describe, expect, it } from "vitest";
import { formatTemplateActiveDevelopersLabel } from "./templates";

describe("formatTemplateActiveDevelopersLabel", () => {
	it("formats singular and plural developer counts", () => {
		expect(formatTemplateActiveDevelopersLabel(1)).toBe("1 developer");
		expect(formatTemplateActiveDevelopersLabel(125)).toBe("125 developers");
	});

	it("formats an unavailable count", () => {
		expect(formatTemplateActiveDevelopersLabel()).toBe("- developers");
	});
});
