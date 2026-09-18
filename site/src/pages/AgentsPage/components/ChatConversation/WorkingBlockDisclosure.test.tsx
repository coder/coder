import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, useState } from "react";
import {
	didPrependIntoBlock,
	WorkingBlockDisclosure,
} from "./WorkingBlockDisclosure";
import type { WorkingBlock } from "./workingBlockGrouping";

const block: WorkingBlock = {
	key: "working:through:message:5",
	liveKey: "working:live:message:1:0",
	rowIndices: [0, 1],
	memberIds: [3, 5],
	startedAt: 0,
	endedAt: 12_000,
	stepCount: 2,
	failedCount: 0,
	isLive: false,
	isPartial: false,
};

const ControlledDisclosure = (
	props: Partial<ComponentProps<typeof WorkingBlockDisclosure>>,
) => {
	const [expanded, setExpanded] = useState(props.expanded ?? false);
	return (
		<WorkingBlockDisclosure
			block={block}
			now={12_000}
			{...props}
			expanded={expanded}
			onExpandedChange={(next) => {
				setExpanded(next);
				props.onExpandedChange?.(next);
			}}
		>
			<ul aria-label="Original tool steps">
				<li>Read src/main.ts</li>
			</ul>
		</WorkingBlockDisclosure>
	);
};

describe("didPrependIntoBlock", () => {
	it("detects older members joining the front", () => {
		expect(didPrependIntoBlock([3, 5], [1, 3, 5])).toBe(true);
	});

	it("detects a merged first row growing under a stable row key", () => {
		expect(didPrependIntoBlock([7, 9], [4, 5, 7, 9])).toBe(true);
	});

	it("ignores unchanged, appended, and replaced members", () => {
		expect(didPrependIntoBlock([3, 5], [3, 5])).toBe(false);
		expect(didPrependIntoBlock([3, 5], [3, 5, 7])).toBe(false);
		expect(didPrependIntoBlock([3, 5], [1, 2])).toBe(false);
	});

	it("ignores the live row becoming its persisted step", () => {
		expect(didPrependIntoBlock([], [7])).toBe(false);
	});
});

describe("WorkingBlockDisclosure", () => {
	it("toggles with Enter and Space and keeps focus on the summary", async () => {
		const user = userEvent.setup();
		const onExpandedChange = vi.fn();
		render(<ControlledDisclosure onExpandedChange={onExpandedChange} />);
		const summary = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});

		await user.tab();
		expect(summary).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(onExpandedChange).toHaveBeenLastCalledWith(true);

		await user.keyboard(" ");
		expect(onExpandedChange).toHaveBeenLastCalledWith(false);
		expect(summary).toHaveFocus();
	});

	it("collapses an expanded block on click", async () => {
		const user = userEvent.setup();
		const onExpandedChange = vi.fn();
		render(
			<ControlledDisclosure expanded onExpandedChange={onExpandedChange} />,
		);

		await user.click(
			screen.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		);
		expect(onExpandedChange).toHaveBeenCalledWith(false);
	});
});
