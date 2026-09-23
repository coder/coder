import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
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
	it("resolves a last seen preset from the URL against the current time", async () => {
		renderWithAuth(<UsersPage />, {
			path: "/deployment/users",
			route: "/deployment/users?filter=status%3Aactive&last_seen=over_90d",
		});

		await waitFor(() =>
			expect(API.getUsers).toHaveBeenCalledWith(
				expect.objectContaining({
					q: 'status:active last_seen_before:"2025-12-12T12:00:00.000Z"',
				}),
				expect.anything(),
			),
		);
	});

	it("stores a picked preset in the URL instead of timestamps", async () => {
		const user = userEvent.setup();
		const { router } = renderWithAuth(<UsersPage />, {
			path: "/deployment/users",
			route: "/deployment/users?filter=status%3Aactive",
		});

		await user.click(await screen.findByRole("button", { name: "Last seen" }));
		await user.click(
			await screen.findByRole("radio", { name: "Over 30 days" }),
		);

		await waitFor(() => {
			const params = router.state.location.search;
			expect(new URLSearchParams(params).get("last_seen")).toBe("over_30d");
			expect(new URLSearchParams(params).get("filter")).toBe("status:active");
		});
	});
});
