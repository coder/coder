import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MarkdownPreviewToggle } from "./MarkdownPreviewToggle";

const renderToggle = (props: { disabledReason?: string } = {}) => {
	const onToggle = vi.fn();
	render(
		<TooltipProvider>
			<MarkdownPreviewToggle
				isRendered={false}
				onToggle={onToggle}
				{...props}
			/>
		</TooltipProvider>,
	);
	return {
		onToggle,
		button: screen.getByRole("button", { name: "Preview Markdown" }),
	};
};

describe("MarkdownPreviewToggle", () => {
	it("calls onToggle on click", async () => {
		const user = userEvent.setup();
		const { button, onToggle } = renderToggle();
		await user.click(button);
		expect(onToggle).toHaveBeenCalledTimes(1);
	});

	it("calls onToggle on Enter and Space", async () => {
		const user = userEvent.setup();
		const { button, onToggle } = renderToggle();
		button.focus();
		await user.keyboard("{Enter}");
		await user.keyboard(" ");
		expect(onToggle).toHaveBeenCalledTimes(2);
	});

	it("ignores activation while a disabled reason is set", async () => {
		const user = userEvent.setup();
		const { button, onToggle } = renderToggle({
			disabledReason: "File too large to preview",
		});
		await user.click(button);
		button.focus();
		await user.keyboard("{Enter}");
		expect(onToggle).not.toHaveBeenCalled();
	});
});
