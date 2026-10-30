import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ModuleCard } from "./ModuleCard";

const renderCard = () => {
	const onSelect = vi.fn();
	renderComponent(
		<ModuleCard
			name="Docker Containers"
			description="Provision Docker containers as Coder workspaces."
			onSelect={onSelect}
		/>,
	);
	return onSelect;
};

describe(ModuleCard.name, () => {
	it("toggles once when the checkbox is clicked", async () => {
		const onSelect = renderCard();
		await userEvent.click(
			screen.getByRole("checkbox", { name: /Docker Containers/ }),
		);
		expect(onSelect).toHaveBeenCalledTimes(1);
	});

	it("toggles when the checkbox is activated with the keyboard", async () => {
		const onSelect = renderCard();
		await userEvent.tab();
		await userEvent.keyboard(" ");
		expect(onSelect).toHaveBeenCalledTimes(1);
	});
});
