import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MCPServerToggleRow } from "./MCPServerToggleRow";

describe("MCPServerToggleRow", () => {
	it("reports the next checked state and labels the switch by its state", async () => {
		const user = userEvent.setup();
		const onCheckedChange = vi.fn();
		const { rerender } = render(
			<MCPServerToggleRow
				icon={null}
				label="github"
				checked={true}
				onCheckedChange={onCheckedChange}
			/>,
		);

		await user.click(screen.getByRole("switch", { name: "Disable github" }));
		expect(onCheckedChange).toHaveBeenLastCalledWith(false);

		rerender(
			<MCPServerToggleRow
				icon={null}
				label="github"
				checked={false}
				onCheckedChange={onCheckedChange}
			/>,
		);
		await user.click(screen.getByRole("switch", { name: "Enable github" }));
		expect(onCheckedChange).toHaveBeenLastCalledWith(true);
	});

	it("disables the switch for a locked server", () => {
		render(
			<MCPServerToggleRow
				icon={null}
				label="forced"
				checked={true}
				locked
				onCheckedChange={vi.fn()}
			/>,
		);
		expect(
			screen.getByRole("switch", { name: "Disable forced" }),
		).toBeDisabled();
	});

	it("renders the action instead of the switch", () => {
		render(
			<MCPServerToggleRow
				icon={null}
				label="linear"
				checked={false}
				onCheckedChange={vi.fn()}
				action={<button type="button">Auth</button>}
			/>,
		);
		expect(screen.getByRole("button", { name: "Auth" })).toBeInTheDocument();
		expect(screen.queryByRole("switch")).not.toBeInTheDocument();
	});
});
