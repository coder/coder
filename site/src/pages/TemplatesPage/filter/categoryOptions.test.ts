import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	getAttributeFilterOptions,
	getOrganizationFilterOptions,
} from "./categoryOptions";

describe("getAttributeFilterOptions", () => {
	it("returns every template attribute as a key:true token", async () => {
		const options = await getAttributeFilterOptions("");

		expect(options.map((option) => option.token)).toEqual([
			"deprecated:true",
			"has-ai-task:true",
			"agents-allowed:true",
			"has_external_agent:true",
		]);
	});

	it("filters attributes by label or value", async () => {
		expect(
			(await getAttributeFilterOptions("deprec")).map((option) => option.token),
		).toEqual(["deprecated:true"]);
		expect(
			(await getAttributeFilterOptions("ai-task")).map(
				(option) => option.token,
			),
		).toEqual(["has-ai-task:true"]);
	});
});

describe("getOrganizationFilterOptions", () => {
	const organizations = [MockDefaultOrganization, MockOrganization2];

	it("maps organizations to name-keyed options", async () => {
		const options = await getOrganizationFilterOptions("", organizations);

		expect(options.map((option) => option.value)).toEqual([
			MockDefaultOrganization.name,
			MockOrganization2.name,
		]);
	});

	it("filters organizations by label or name", async () => {
		const options = await getOrganizationFilterOptions(
			MockOrganization2.name,
			organizations,
		);

		expect(options.map((option) => option.value)).toEqual([
			MockOrganization2.name,
		]);
	});
});
