import { fireEvent, render, screen } from "@testing-library/react";
import { Command, CommandInput, CommandItem, CommandList } from "./Command";

const renderCommand = () => {
	render(
		<Command label="Fruit">
			<CommandInput placeholder="Search" />
			<CommandList>
				<CommandItem>Apple</CommandItem>
				<CommandItem>Banana</CommandItem>
			</CommandList>
		</Command>,
	);
	const input = screen.getByPlaceholderText("Search");
	const root = input.closest("[cmdk-root]");
	if (!root) {
		throw new Error("cmdk root not found");
	}
	return { input, root };
};

describe("Command", () => {
	it("marks keyboard highlights so only they show the ring", () => {
		const { input, root } = renderCommand();

		fireEvent.pointerMove(screen.getByText("Banana"));
		expect(root).toHaveAttribute("data-highlight-source", "pointer");

		fireEvent.keyDown(input, { key: "ArrowDown" });
		expect(root).toHaveAttribute("data-highlight-source", "keyboard");

		fireEvent.pointerMove(screen.getByText("Apple"));
		expect(root).toHaveAttribute("data-highlight-source", "pointer");
	});

	it("marks keyboard highlights driven by keys outside the root", () => {
		const { root } = renderCommand();

		fireEvent.pointerMove(screen.getByText("Banana"));
		expect(root).toHaveAttribute("data-highlight-source", "pointer");

		fireEvent.keyDown(document.body, { key: "ArrowDown" });
		expect(root).toHaveAttribute("data-highlight-source", "keyboard");
	});

	it("starts from the last input on the page when it takes focus", () => {
		const { input, root } = renderCommand();

		fireEvent.pointerDown(document.body);
		fireEvent.focus(input);
		expect(root).toHaveAttribute("data-highlight-source", "pointer");

		fireEvent.keyDown(document.body, { key: "Tab" });
		fireEvent.focus(input);
		expect(root).toHaveAttribute("data-highlight-source", "keyboard");
	});
});
