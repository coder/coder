import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSearchParams } from "react-router";
import { useFilter } from "#/components/Filter/Filter";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ConnectionLogFilter } from "./ConnectionLogFilter";

const Harness = ({ onChange }: { onChange: (query: string) => void }) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: onChange,
	});
	return <ConnectionLogFilter filter={filter} />;
};

describe("ConnectionLogFilter", () => {
	it("replaces the query with the active SSH preset", async () => {
		const user = userEvent.setup({ skipHover: true });
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=status:completed%20workspace_owner:alice",
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.click(
			await screen.findByRole("option", { name: "Active SSH connections" }),
		);
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:ongoing method:ssh"),
		);
	});

	it("replaces status without losing the legacy type or advanced fields", async () => {
		const user = userEvent.setup({ skipHover: true });
		const onChange = vi.fn();
		renderWithAuth(<Harness onChange={onChange} />, {
			route: "/?filter=status:completed%20type:ssh%20app:code-server",
		});
		await user.click(await screen.findByRole("button", { name: "Filters" }));
		await user.hover(await screen.findByRole("option", { name: "Status" }));
		await user.click(await screen.findByRole("button", { name: "Ongoing" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"status:ongoing type:ssh app:code-server",
			),
		);
	});
});
