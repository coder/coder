import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { MockNoPermissions, MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { TemplatesFilter } from "./TemplatesFilter";

const TemplatesFilterHarness = ({
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
	return <TemplatesFilter filter={filter} error={undefined} />;
};

describe("TemplatesFilter", () => {
	it.each([
		["Deprecated", "deprecated:true"],
		["Compatibility mode", "compatibility_mode:true"],
		["Agents allowed", "agents-allowed:true"],
		["Has external agent", "has_external_agent:true"],
	])("emits the query for %s", async (label, token) => {
		const user = userEvent.setup({ skipHover: true });
		const onUpdate = vi.fn();
		renderWithAuth(<TemplatesFilterHarness onUpdate={onUpdate} />);

		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.hover(
			await screen.findByRole("option", { name: /^Attributes/ }),
		);
		await user.click(await screen.findByRole("button", { name: label }));

		await waitFor(() => expect(onUpdate).toHaveBeenLastCalledWith(token));
	});

	it("allows ordinary users to select themselves without listing users", async () => {
		const listUsers = vi.fn(() => HttpResponse.json({ users: [] }));
		server.use(
			http.post("/api/v2/authcheck", () =>
				HttpResponse.json(MockNoPermissions),
			),
			http.get("/api/v2/users", listUsers),
		);
		const user = userEvent.setup({ skipHover: true });
		const onUpdate = vi.fn();
		renderWithAuth(<TemplatesFilterHarness onUpdate={onUpdate} />);

		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.hover(await screen.findByRole("option", { name: /^Author/ }));
		await user.click(
			await screen.findByRole("button", {
				name: `${MockUserOwner.username} (you)`,
			}),
		);

		await waitFor(() => expect(onUpdate).toHaveBeenLastCalledWith("author:me"));
		expect(listUsers).not.toHaveBeenCalled();
	});

	it("removes an author chip while preserving the attribute filter", async () => {
		const user = userEvent.setup({ skipHover: true });
		const onUpdate = vi.fn();
		renderWithAuth(
			<TemplatesFilterHarness
				onUpdate={onUpdate}
				initialQuery="author:me deprecated:true"
			/>,
		);
		await user.click(
			await screen.findByRole("button", { name: "Remove author:me" }),
		);
		await waitFor(() =>
			expect(onUpdate).toHaveBeenLastCalledWith("deprecated:true"),
		);
	});
});
