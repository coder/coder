import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { API } from "#/api/api";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ListSessionsFilter } from "./ListSessionsFilter";

const rangeQuery =
	'started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"';
const bedrockModel = "us.anthropic.claude-3-5-sonnet-20241022-v2:0";

// Feeds `filter.update` back into the value the way `useFilter` does.
const ListSessionsFilterHarness = ({
	onUpdate,
	initialQuery,
}: {
	onUpdate: (query: string) => void;
	initialQuery: string;
}) => {
	const [query, setQuery] = useState(initialQuery);
	const update: UseFilterResult["update"] = (next) => {
		const nextQuery = typeof next === "string" ? next : "";
		onUpdate(nextQuery);
		setQuery(nextQuery);
	};
	const filter: UseFilterResult = {
		query,
		values: {},
		used: query.length > 0,
		update,
		debounceUpdate: update,
		cancelDebounce: () => {},
	};
	return (
		<ListSessionsFilter
			filter={filter}
			timeRange={{
				start: new Date("2026-08-12T15:00:00Z"),
				end: new Date("2026-08-13T15:00:00Z"),
			}}
			onTimeRangeChange={vi.fn()}
		/>
	);
};

describe("ListSessionsFilter", () => {
	beforeEach(() => {
		vi.spyOn(API, "getAIBridgeProviders").mockResolvedValue([]);
		vi.spyOn(API, "getAIBridgeClients").mockResolvedValue([]);
		vi.spyOn(API, "getAIBridgeModels").mockResolvedValue([bedrockModel]);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("keeps the time range when a model with a colon is picked", async () => {
		const user = userEvent.setup({ skipHover: true });
		const onUpdate = vi.fn();
		renderWithAuth(
			<ListSessionsFilterHarness
				onUpdate={onUpdate}
				initialQuery={rangeQuery}
			/>,
		);

		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.hover(await screen.findByRole("option", { name: "Model" }));
		await user.click(await screen.findByRole("button", { name: bedrockModel }));

		await waitFor(() =>
			expect(onUpdate).toHaveBeenLastCalledWith(
				`model:"${bedrockModel}" ${rangeQuery}`,
			),
		);
	});
});
