import { renderComponent } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TaskActionButton } from "./TaskActionButton";

describe("TaskActionButton", () => {
	it("renders with correct aria label for each action", async () => {
		const { rerender } = renderComponent(
			<TaskActionButton action="pause" onClick={() => {}} />,
		);
		expect(
			await screen.findByRole("button", { name: /pause task/i }),
		).toBeInTheDocument();

		rerender(<TaskActionButton action="resume" onClick={() => {}} />);
		expect(
			await screen.findByRole("button", { name: /resume task/i }),
		).toBeInTheDocument();
	});

	it("is disabled when disabled or loading", async () => {
		const { rerender } = renderComponent(
			<TaskActionButton action="pause" disabled onClick={() => {}} />,
		);
		expect(
			await screen.findByRole("button", { name: /pause task/i }),
		).toBeDisabled();

		rerender(<TaskActionButton action="pause" loading onClick={() => {}} />);
		expect(
			await screen.findByRole("button", { name: /pause task/i }),
		).toBeDisabled();
	});

	it("calls onClick when clicked and stops propagation", async () => {
		const user = userEvent.setup();
		const parentClick = vi.fn();
		const buttonClick = vi.fn();

		renderComponent(
			// biome-ignore lint/a11y/useKeyWithClickEvents: Test-only element
			<div onClick={parentClick}>
				<TaskActionButton action="pause" onClick={buttonClick} />
			</div>,
		);

		await user.click(
			await screen.findByRole("button", { name: /pause task/i }),
		);

		expect(buttonClick).toHaveBeenCalledTimes(1);
		expect(parentClick).not.toHaveBeenCalled();
	});

	it("does not call onClick when disabled", async () => {
		const user = userEvent.setup();
		const handleClick = vi.fn();
		renderComponent(
			<TaskActionButton action="pause" disabled onClick={handleClick} />,
		);

		await user.click(
			await screen.findByRole("button", { name: /pause task/i }),
		);

		expect(handleClick).not.toHaveBeenCalled();
	});
});
