import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { usersQueryKey } from "#/api/queries/users";
import type { UsersRequest } from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import UsersPage from "./UsersPage";

const now = new Date("2026-03-12T12:00:00.000Z");

beforeEach(() => {
	vi.useFakeTimers({ now, toFake: ["Date"] });
	vi.spyOn(API, "getUsers").mockResolvedValue({
		users: [MockUserOwner],
		count: 1,
	});
});

afterEach(() => {
	vi.useRealTimers();
});

describe("UsersPage", () => {
	it("resolves relative last seen values from the URL against the current time", async () => {
		const filter =
			'status:active last_seen_after:"1970-01-01T00:00:00.000Z" last_seen_before:-90d';
		const { queryClient } = renderWithAuth(<UsersPage />, {
			path: "/deployment/users",
			route: `/deployment/users?filter=${encodeURIComponent(filter)}`,
		});

		await waitFor(() =>
			expect(API.getUsers).toHaveBeenCalledWith(
				expect.objectContaining({
					q: 'status:active last_seen_after:"1970-01-01T00:00:00.000Z" last_seen_before:"2025-12-12T12:00:00.000Z"',
				}),
				expect.anything(),
			),
		);

		// The query key keeps the relative values, so it stays stable as time
		// passes and each fetch resolves them again.
		const keyQueries = queryClient
			.getQueryCache()
			.findAll({ queryKey: usersQueryKey })
			.map((query) => (query.queryKey[1] as UsersRequest).q);
		expect(keyQueries).toContain(filter);
	});

	it("stores a picked preset in the filter as relative values", async () => {
		const user = userEvent.setup();
		const { router } = renderWithAuth(<UsersPage />, {
			path: "/deployment/users",
			route: "/deployment/users?filter=status%3Aactive",
		});

		await user.click(await screen.findByRole("button", { name: "Last seen" }));
		await user.click(
			await screen.findByRole("radio", { name: "Over 30 days" }),
		);

		await waitFor(() =>
			expect(
				new URLSearchParams(router.state.location.search).get("filter"),
			).toBe(
				'status:active last_seen_after:"1970-01-01T00:00:00.000Z" last_seen_before:-30d',
			),
		);
		expect(screen.getByRole("button", { name: /Over 30 days/ })).toBeVisible();
	});
});
