import { describe, expect, it } from "vitest";
import { customizationsComplete } from "./TemplateCustomizationsStep";

describe("customizationsComplete", () => {
	it("requires a non-empty name and organization", () => {
		expect(
			customizationsComplete({
				name: "",
				organizationId: "org-123",
				hasProvisioners: true,
			}),
		).toBe(false);
		expect(
			customizationsComplete({
				name: "my-template",
				organizationId: undefined,
				hasProvisioners: true,
			}),
		).toBe(false);
		expect(
			customizationsComplete({
				name: "my-template",
				organizationId: "",
				hasProvisioners: true,
			}),
		).toBe(false);
	});

	it("blocks create when the organization has no provisioners", () => {
		expect(
			customizationsComplete({
				name: "my-template",
				organizationId: "org-123",
				hasProvisioners: false,
			}),
		).toBe(false);
	});

	it("allows create when name and organization are set", () => {
		expect(
			customizationsComplete({
				name: "my-template",
				organizationId: "org-123",
				hasProvisioners: true,
			}),
		).toBe(true);
		expect(
			customizationsComplete({
				name: "my-template",
				organizationId: "org-123",
				hasProvisioners: undefined,
			}),
		).toBe(true);
	});
});
