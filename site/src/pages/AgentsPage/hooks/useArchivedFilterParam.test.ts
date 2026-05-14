import { act, waitFor } from "@testing-library/react";
import { renderHookWithAuth } from "#/testHelpers/hooks";
import { useArchivedFilterParam } from "./useArchivedFilterParam";

describe(useArchivedFilterParam.name, () => {
	describe("parsing the URL param", () => {
		it.each([
			{ route: "/agents", expected: "active" },
			{ route: "/agents?archived=active", expected: "active" },
			{ route: "/agents?archived=archived", expected: "archived" },
			{ route: "/agents?archived=garbage", expected: "active" },
		])("returns $expected for $route", async ({ route, expected }) => {
			const { result } = await renderHookWithAuth(
				() => useArchivedFilterParam(),
				{ routingOptions: { path: "/agents", route } },
			);

			expect(result.current[0]).toEqual(expected);
		});
	});

	describe("setting the filter", () => {
		it("writes ?archived=archived when set to 'archived'", async () => {
			const { result, getLocationSnapshot } = await renderHookWithAuth(
				() => useArchivedFilterParam(),
				{ routingOptions: { path: "/agents", route: "/agents" } },
			);

			act(() => result.current[1]("archived"));
			await waitFor(() => expect(result.current[0]).toEqual("archived"));

			const { search } = getLocationSnapshot();
			expect(search.get("archived")).toEqual("archived");
		});

		it("removes the param when set to 'active' (does not write archived=active)", async () => {
			const { result, getLocationSnapshot } = await renderHookWithAuth(
				() => useArchivedFilterParam(),
				{
					routingOptions: {
						path: "/agents",
						route: "/agents?archived=archived",
					},
				},
			);

			act(() => result.current[1]("active"));
			await waitFor(() => expect(result.current[0]).toEqual("active"));

			const { search } = getLocationSnapshot();
			expect(search.get("archived")).toEqual(null);
		});

		it("removes the param when set to 'active' from a clean URL (idempotent)", async () => {
			const { result, getLocationSnapshot } = await renderHookWithAuth(
				() => useArchivedFilterParam(),
				{ routingOptions: { path: "/agents", route: "/agents" } },
			);

			act(() => result.current[1]("active"));
			await waitFor(() => expect(result.current[0]).toEqual("active"));

			const { search } = getLocationSnapshot();
			expect(search.get("archived")).toEqual(null);
		});
	});
});
