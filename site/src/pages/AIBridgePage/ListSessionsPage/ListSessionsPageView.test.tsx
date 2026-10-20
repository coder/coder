import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockSession } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { ListSessionsPageView } from "./ListSessionsPageView";
import { sessionColumnViews } from "./sessionColumns";
import { sessionColumnsStorageKey } from "./useSessionColumns";

const userId = "user-1";

const renderView = (viewUserId = userId) =>
	renderWithAuth(
		<ListSessionsPageView
			userId={viewUserId}
			isLoading={false}
			isFetching={false}
			isAISessionsEntitled
			isAISessionsEnabled
			sessions={[MockSession]}
			sessionsQuery={mockSuccessResult}
			filterProps={{
				filter: {
					query: "",
					values: {},
					used: false,
					update: vi.fn(),
					debounceUpdate: vi.fn(),
					cancelDebounce: vi.fn(),
				},
				timeRange: {
					start: new Date("2026-08-12T15:00:00Z"),
					end: new Date("2026-08-13T15:00:00Z"),
					preset: "last_24h",
				},
				onTimeRangeChange: vi.fn(),
			}}
		/>,
	);

const savedSettings = (id: string) =>
	JSON.parse(localStorage.getItem(sessionColumnsStorageKey(id)) ?? "null");

const openColumnsMenu = async () => {
	const user = userEvent.setup();
	await user.click(await screen.findByRole("button", { name: "Columns" }));
	return user;
};

describe("ListSessionsPageView columns", () => {
	beforeEach(() => {
		localStorage.clear();
		MockResizeObserver.reset();
		vi.stubGlobal("ResizeObserver", MockResizeObserver);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("saves the selected view for the signed-in user", async () => {
		renderView();
		const user = await openColumnsMenu();

		await user.click(screen.getByRole("button", { name: "Security" }));

		expect(savedSettings(userId)).toEqual({ view: "security" });
		expect(savedSettings("user-2")).toBeNull();
	});

	it("saves changes on top of the settings from an earlier visit", async () => {
		localStorage.setItem(
			sessionColumnsStorageKey(userId),
			JSON.stringify({ view: "security" }),
		);
		renderView();
		const user = await openColumnsMenu();

		await user.click(screen.getByRole("switch", { name: "Model" }));

		expect(savedSettings(userId).shown).toEqual([
			...sessionColumnViews[1].settings.shown,
			"model",
		]);
	});
});
