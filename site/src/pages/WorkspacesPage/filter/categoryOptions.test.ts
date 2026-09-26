import {
	getAttributeFilterOptions,
	getSelfUserFilterOptions,
	getStatusFilterOptions,
	getUserFilterOptions,
	type OptionsQueryClient,
} from "./categoryOptions";

const fakeQueryClient = <T>(data: T): OptionsQueryClient => ({
	fetchQuery: (async () => data) as OptionsQueryClient["fetchQuery"],
});

describe("getUserFilterOptions", () => {
	const me = { username: "alice", avatar_url: "/alice.png" };

	it("puts the current user first and commits the me sentinel", async () => {
		const queryClient = fakeQueryClient({ users: [] });

		const options = await getUserFilterOptions("", me, queryClient);

		expect(options[0]).toMatchObject({ label: "alice (you)", value: "me" });
	});

	it("drops the current user from the fetched list to avoid a duplicate", async () => {
		const queryClient = fakeQueryClient({
			users: [
				{ username: "alice", avatar_url: "/alice.png" },
				{ username: "bob", avatar_url: "/bob.png" },
			],
		});

		const options = await getUserFilterOptions("", me, queryClient);

		expect(options.map((option) => option.value)).toEqual(["me", "bob"]);
	});

	it("lists the current user only when the query matches it", async () => {
		const queryClient = fakeQueryClient({ users: [] });

		expect(await getUserFilterOptions("ali", me, queryClient)).toHaveLength(1);
		expect(await getUserFilterOptions("zzz", me, queryClient)).toHaveLength(0);
	});

	it("lists the current user when the users API matches it by name or email", async () => {
		const queryClient = fakeQueryClient({
			users: [
				{ username: "alice", avatar_url: "/alice.png" },
				{ username: "bob", avatar_url: "/bob.png" },
			],
		});

		const options = await getUserFilterOptions("smith", me, queryClient);

		expect(options.map((option) => option.value)).toEqual(["me", "bob"]);
	});
});

describe("getSelfUserFilterOptions", () => {
	const me = { username: "alice", avatar_url: "/alice.png" };

	it("returns only the current user without fetching", async () => {
		const options = await getSelfUserFilterOptions("", me);

		expect(options).toMatchObject([{ label: "alice (you)", value: "me" }]);
	});

	it("matches the self option by the me sentinel and username", async () => {
		expect(await getSelfUserFilterOptions("me", me)).toHaveLength(1);
		expect(await getSelfUserFilterOptions("ali", me)).toHaveLength(1);
		expect(await getSelfUserFilterOptions("bob", me)).toHaveLength(0);
	});
});

describe("getAttributeFilterOptions", () => {
	it("hides the dormant attribute without the entitlement", async () => {
		const options = await getAttributeFilterOptions("", {
			canFilterDormant: false,
		});

		expect(options.map((option) => option.token)).toEqual(["outdated:true"]);
	});

	it("shows the dormant attribute with the entitlement", async () => {
		const options = await getAttributeFilterOptions("", {
			canFilterDormant: true,
		});

		expect(options.map((option) => option.token)).toEqual([
			"outdated:true",
			"dormant:true",
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
