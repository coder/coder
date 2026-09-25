import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { MockNoPermissions } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { WorkspacesFilter } from "./WorkspacesFilter";

// Feeds `filter.update` back into the value the way `useFilter` does.
const WorkspacesFilterHarness = ({
	onUpdate,
	initialQuery = "",
}: {
	onUpdate: (query: string) => void;
	initialQuery?: string;
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

	// Removing the pill narrows the chip, which works only when `user:me`
	// parsed as an Owner chip with the scope pill rather than as free text.
	it.each([
		["a user who can list others", undefined],
		["a user who cannot list others", MockNoPermissions],
	])(
		"renders the default user:me as a scoped Owner chip for %s",
		async (_, permissions) => {
			if (permissions) {
				server.use(
					http.post("/api/v2/authcheck", () => HttpResponse.json(permissions)),
				);
			}
			const user = userEvent.setup();
			const onUpdate = vi.fn();
			renderWithAuth(
				<WorkspacesFilterHarness onUpdate={onUpdate} initialQuery="user:me" />,
			);

			await user.click(
				await screen.findByRole("button", {
					name: "Hide workspaces shared with me",
				}),
			);

			await waitFor(() =>
				expect(onUpdate).toHaveBeenLastCalledWith("owner:me"),
			);
		},
	);
});
