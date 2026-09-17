import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type * as MessageScrollerModule from "#/vendor/message-scroller";
import { ToolCall } from "./ToolCall";

// The layout-intent signal is a viewport-scoped context hook from the vendored
// scroller. Mocking it keeps this a unit test of ToolCall's toggle boundary:
// the spy stands in for the controller's userLayoutIntent, and the real
// hook-to-controller wiring is covered by the Storybook scroll stories.
vi.mock("#/vendor/message-scroller", async (importOriginal) => {
	const actual = await importOriginal<typeof MessageScrollerModule>();
	return {
		...actual,
		useMessageScrollerLayoutIntent: vi.fn(),
	};
});

const layoutIntent = vi.fn();

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
	it("signals the scroller layout intent before the view changes", async () => {
		const { useMessageScrollerLayoutIntent } = await import(
			"#/vendor/message-scroller"
		);
		vi.mocked(useMessageScrollerLayoutIntent).mockReturnValue(layoutIntent);
		layoutIntent.mockClear();

		const user = userEvent.setup();
		const onViewChange = vi.fn();

		render(<ToggleHarness view="collapsed" onViewChange={onViewChange} />);

		await user.click(screen.getByRole("button", { name: "Read config.yaml" }));

		// The intent fires exactly once, before the view-change callback runs,
		// so the scroller's latch precedes the layout change it reacts to.
		expect(layoutIntent).toHaveBeenCalledTimes(1);
		expect(layoutIntent.mock.invocationCallOrder[0]).toBeLessThan(
			onViewChange.mock.invocationCallOrder[0],
		);
		expect(onViewChange).toHaveBeenCalledWith("expanded");
	});

	it("signals the layout intent on collapse toggles too", async () => {
		const user = userEvent.setup();
		const onViewChange = vi.fn();

		render(<ToggleHarness view="expanded" onViewChange={onViewChange} />);

		await user.click(screen.getByRole("button", { name: "Read config.yaml" }));

		expect(layoutIntent).toHaveBeenCalledTimes(1);
		expect(onViewChange).toHaveBeenCalledWith("collapsed");
	});

	it("signals the layout intent on keyboard activation", async () => {
		const user = userEvent.setup();
		const onViewChange = vi.fn();

		render(<ToggleHarness view="collapsed" onViewChange={onViewChange} />);

		const toggle = screen.getByRole("button", { name: "Read config.yaml" });
		toggle.focus();
		await user.keyboard("{Enter}");

		expect(layoutIntent).toHaveBeenCalledTimes(1);
		expect(onViewChange).toHaveBeenCalledWith("expanded");
	});
});
