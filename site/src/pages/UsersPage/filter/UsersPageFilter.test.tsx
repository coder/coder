import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { render } from "#/testHelpers/renderHelpers";
import { UsersPageFilter } from "./UsersPageFilter";

const lastSeenQuery =
	'last_seen_after:"2026-03-05T12:00:00.000Z" last_seen_before:"2026-03-12T12:00:00.000Z"';

const renderFilter = (
	query: string,
	lastSeen: DateTimeRangeValue = {
		start: new Date("2026-03-05T12:00:00.000Z"),
		end: new Date("2026-03-12T12:00:00.000Z"),
		preset: "last_7d",
	},
) => {
	const update = vi.fn();
	const onLastSeenChange = vi.fn();
	const filter: UseFilterResult = {
		query,
		update,
		debounceUpdate: vi.fn(),
		cancelDebounce: vi.fn(),
		used: query.length > 0,
		values: {},
	};
	render(
		<UsersPageFilter
			filter={filter}
			lastSeen={lastSeen}
			onLastSeenChange={onLastSeenChange}
		/>,
	);
	return { update, onLastSeenChange };
};

beforeEach(() => {
	vi.spyOn(API, "getRoles").mockResolvedValue([]);
});

describe("UsersPageFilter", () => {
	it("keeps the last seen range when a user type is picked", async () => {
		const user = userEvent.setup();
		const { update } = renderFilter(`role:owner ${lastSeenQuery}`);

		await user.click(screen.getByRole("button", { name: "Toggle filters" }));
		await user.click(
			await screen.findByRole("option", { name: /Service accounts/ }),
		);

		await waitFor(() =>
			expect(update).toHaveBeenLastCalledWith(
				`role:owner service_account:true ${lastSeenQuery}`,
			),
		);
	});

	it("reports a last seen preset picked from the default All time state", async () => {
		const user = userEvent.setup();
		const { onLastSeenChange } = renderFilter("", {
			start: new Date(0),
			end: new Date("2026-03-12T12:00:00.000Z"),
			preset: "all_time",
		});

		await user.click(screen.getByRole("button", { name: "Last seen" }));
		await user.click(await screen.findByRole("radio", { name: "Last 7 days" }));

		expect(onLastSeenChange).toHaveBeenCalledWith(
			expect.objectContaining({ preset: "last_7d" }),
		);
	});

	it("replaces the selected user type instead of adding a second one", async () => {
		const user = userEvent.setup();
		const { update } = renderFilter("service_account:false");

		await user.click(screen.getByRole("button", { name: "Toggle filters" }));
		await user.click(
			await screen.findByRole("option", { name: /Service accounts/ }),
		);

		await waitFor(() =>
			expect(update).toHaveBeenLastCalledWith("service_account:true"),
		);
	});
});
