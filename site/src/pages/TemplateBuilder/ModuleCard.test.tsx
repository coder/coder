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
			detailsUrl="https://registry.coder.com/modules/docker"
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

	it("toggles once when the card name is clicked", async () => {
		const onSelect = renderCard();
		await userEvent.click(screen.getByText("Docker Containers"));
		expect(onSelect).toHaveBeenCalledTimes(1);
	});

	it("names the checkbox and details link after the card", () => {
		renderCard();
		expect(
			screen.getByRole("checkbox", {
				// jsdom drops the leading space of the sr-only text.
				name: /^Docker Containers\s*\(official\)$/,
			}),
		).toHaveAccessibleDescription(
			"Provision Docker containers as Coder workspaces.",
		);
		expect(
			screen.getByRole("link", { name: /Docker Containers/ }),
		).toHaveAccessibleName(
			/^View details\s*for Docker Containers\s*\(opens in new tab\)$/,
		);
	});

	it("toggles when the checkbox is activated with the keyboard", async () => {
		const onSelect = renderCard();
		await userEvent.tab();
		await userEvent.keyboard(" ");
		expect(onSelect).toHaveBeenCalledTimes(1);
	});
});
