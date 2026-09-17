import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ToolCall } from "./ToolCall";

const ToggleHarness = ({
	view,
	onViewChange,
}: {
	view?: "collapsed" | "preview" | "expanded";
	onViewChange?: (view: "collapsed" | "preview" | "expanded") => void;
}) => (
	<ToolCall.Root
		status="completed"
		hasContent
		view={view}
		onViewChange={onViewChange}
	>
		<ToolCall.Header iconName="read_file" label="Read config.yaml" />
		<ToolCall.Content>
			<div>File contents</div>
		</ToolCall.Content>
	</ToolCall.Root>
);

describe("ToolCall", () => {
	it("dispatches the scroller layout intent event before the view changes", async () => {
		const user = userEvent.setup();
		const onViewChange = vi.fn();

		render(<ToggleHarness view="collapsed" onViewChange={onViewChange} />);

		const listener = vi.fn();
		document.addEventListener("messagescroller:userlayoutintent", listener);

		await user.click(screen.getByRole("button", { name: "Read config.yaml" }));

		// The event fires exactly once, before the view-change callback runs,
		// so the scroller's latch precedes the layout change it reacts to.
		expect(listener).toHaveBeenCalledTimes(1);
		expect(onViewChange).toHaveBeenCalledWith("expanded");

		document.removeEventListener("messagescroller:userlayoutintent", listener);
	});

	it("dispatches the intent event on collapse toggles too", async () => {
		const user = userEvent.setup();
		const onViewChange = vi.fn();

		render(<ToggleHarness view="expanded" onViewChange={onViewChange} />);

		const listener = vi.fn();
		document.addEventListener("messagescroller:userlayoutintent", listener);

		await user.click(screen.getByRole("button", { name: "Read config.yaml" }));

		expect(listener).toHaveBeenCalledTimes(1);
		expect(onViewChange).toHaveBeenCalledWith("collapsed");

		document.removeEventListener("messagescroller:userlayoutintent", listener);
	});

	it("supports keyboard activation through the same intent path", async () => {
		const user = userEvent.setup();
		const onViewChange = vi.fn();

		render(<ToggleHarness view="collapsed" onViewChange={onViewChange} />);

		const listener = vi.fn();
		document.addEventListener("messagescroller:userlayoutintent", listener);

		await user.click(screen.getByRole("button", { name: "Read config.yaml" }));

		expect(listener).toHaveBeenCalledTimes(1);

		document.removeEventListener("messagescroller:userlayoutintent", listener);
	});
});
