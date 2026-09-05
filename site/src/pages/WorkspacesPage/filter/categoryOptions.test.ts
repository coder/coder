import {
	getAttributeFilterOptions,
	getOwnerFilterOptions,
	getSelfOwnerFilterOptions,
	getStatusFilterOptions,
	type OptionsQueryClient,
} from "./categoryOptions";

const fakeQueryClient = <T>(data: T): OptionsQueryClient => ({
	fetchQuery: (async () => data) as OptionsQueryClient["fetchQuery"],
});

describe("getOwnerFilterOptions", () => {
	const me = { username: "alice", avatar_url: "/alice.png" };

	it("puts the current user first and commits owner:me", async () => {
		const queryClient = fakeQueryClient({ users: [] });

		const options = await getOwnerFilterOptions("", me, queryClient);

		expect(options[0]).toMatchObject({ label: "alice (you)", value: "me" });
	});

	it("drops the current user from the fetched list to avoid a duplicate", async () => {
		const queryClient = fakeQueryClient({
			users: [
				{ username: "alice", avatar_url: "/alice.png" },
				{ username: "bob", avatar_url: "/bob.png" },
			],
		});

		const options = await getOwnerFilterOptions("", me, queryClient);

		expect(options.map((option) => option.value)).toEqual(["me", "bob"]);
	});
});

describe("getSelfOwnerFilterOptions", () => {
	const me = { username: "alice", avatar_url: "/alice.png" };

	it("returns only the current user without fetching", async () => {
		const options = await getSelfOwnerFilterOptions("", me);

		expect(options).toMatchObject([{ label: "alice (you)", value: "me" }]);
	});

	it("matches the self option by the me sentinel and username", async () => {
		expect(await getSelfOwnerFilterOptions("me", me)).toHaveLength(1);
		expect(await getSelfOwnerFilterOptions("ali", me)).toHaveLength(1);
		expect(await getSelfOwnerFilterOptions("bob", me)).toHaveLength(0);
	});
});

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
