import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { WorkspacesFilter } from "./WorkspacesFilter";

// Feeds `filter.update` back into the value the way `useFilter` does.
const WorkspacesFilterHarness = ({
	onUpdate,
}: {
	onUpdate: (query: string) => void;
}) => {
	const [query, setQuery] = useState("");
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
	return <WorkspacesFilter filter={filter} error={undefined} />;
};

describe("WorkspacesFilter", () => {
	it("emits a status query when Running is picked", async () => {
		const user = userEvent.setup();
		const onUpdate = vi.fn();
		renderWithAuth(<WorkspacesFilterHarness onUpdate={onUpdate} />);

		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(await screen.findByRole("option", { name: /running/i }));

		await waitFor(() =>
			expect(onUpdate).toHaveBeenLastCalledWith("status:running"),
		);
	});
});
