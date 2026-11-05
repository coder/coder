import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SelectFilter } from "./SelectFilter";

it("selects an option with the keyboard alone", async () => {
	const user = userEvent.setup();
	const onSelect = vi.fn();
	render(
		<SelectFilter
			label="Select a status"
			placeholder="All statuses"
			options={[
				{ label: "Active", value: "active" },
				{ label: "Dormant", value: "dormant" },
			]}
			onSelect={onSelect}
		/>,
	);

	await user.tab();
	await user.keyboard("{Enter}");
	await screen.findByRole("option", { name: "Active" });
	await user.keyboard("{ArrowDown}{Enter}");
	expect(onSelect).toHaveBeenCalledWith({ label: "Dormant", value: "dormant" });
});
