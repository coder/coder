import {
	getAttributeFilterOptions,
	getStatusFilterOptions,
} from "./categoryOptions";

describe("getAttributeFilterOptions", () => {
	it("hides the dormant attribute without the entitlement", async () => {
		const options = await getAttributeFilterOptions("", {
			canFilterDormant: false,
		});

		expect(options.map((option) => option.token)).toEqual([
			"outdated:true",
			"shared:true",
		]);
	});

	it("shows the dormant attribute with the entitlement", async () => {
		const options = await getAttributeFilterOptions("", {
			canFilterDormant: true,
		});

		expect(options.map((option) => option.token)).toEqual([
			"outdated:true",
			"dormant:true",
			"shared:true",
		]);
	});

	it("filters attributes by label or value", async () => {
		const options = await getAttributeFilterOptions("dorm", {
			canFilterDormant: true,
		});

		expect(options.map((option) => option.token)).toEqual(["dormant:true"]);
	});
});

describe("getStatusFilterOptions", () => {
	it("filters statuses by the typed query", async () => {
		const options = await getStatusFilterOptions("run");

		expect(options.map((option) => option.value)).toEqual(["running"]);
	});
});
