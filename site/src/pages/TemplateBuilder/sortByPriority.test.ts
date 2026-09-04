import { sortByPriority } from "./sortByPriority";

describe("sortByPriority", () => {
	it("moves prioritized ids to the front in priority order", () => {
		const items = [
			{ id: "aws-linux" },
			{ id: "docker" },
			{ id: "kubernetes" },
			{ id: "quickstart" },
		];
		const sorted = sortByPriority(items, ["quickstart", "docker"]);
		expect(sorted.map((item) => item.id)).toEqual([
			"quickstart",
			"docker",
			"aws-linux",
			"kubernetes",
		]);
	});

	it("keeps unlisted items in their original relative order", () => {
		const items = [{ id: "b" }, { id: "a" }, { id: "c" }];
		const sorted = sortByPriority(items, []);
		expect(sorted.map((item) => item.id)).toEqual(["b", "a", "c"]);
	});

	it("orders prioritized items first and is stable among the unlisted", () => {
		const items = [
			{ id: "first-unlisted" },
			{ id: "docker" },
			{ id: "second-unlisted" },
			{ id: "quickstart" },
		];
		const sorted = sortByPriority(items, ["quickstart", "docker"]);
		expect(sorted.map((item) => item.id)).toEqual([
			"quickstart",
			"docker",
			"first-unlisted",
			"second-unlisted",
		]);
	});

	it("ignores priority ids that are not present in items", () => {
		const items = [{ id: "a" }, { id: "quickstart" }];
		const sorted = sortByPriority(items, ["missing", "quickstart"]);
		expect(sorted.map((item) => item.id)).toEqual(["quickstart", "a"]);
	});

	it("does not mutate the input array", () => {
		const items = [{ id: "docker" }, { id: "quickstart" }];
		const snapshot = items.map((item) => item.id);
		sortByPriority(items, ["quickstart"]);
		expect(items.map((item) => item.id)).toEqual(snapshot);
	});

	it("returns an empty array unchanged", () => {
		expect(sortByPriority([], ["quickstart"])).toEqual([]);
	});
});
