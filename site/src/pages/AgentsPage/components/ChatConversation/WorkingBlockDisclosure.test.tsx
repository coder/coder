import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
	flushLayout,
	history,
	nextRow,
	prompt,
	renderTranscript,
	row,
	setupMessageScrollerLayout,
	viewport,
	visibleRowsAfterLayout,
} from "#/testHelpers/messageScrollerLayout";
import { MessageScroller } from "#/vendor/message-scroller";
import { FIXTURE_NOW, MockWorkingBlock } from "./storyFixtures";
import {
	didPrependIntoBlock,
	formatLiveElapsed,
	WorkingBlockDisclosure,
} from "./WorkingBlockDisclosure";

const ControlledDisclosure = (
	props: Partial<React.ComponentProps<typeof WorkingBlockDisclosure>>,
) => {
	const [uncontrolledExpanded, setUncontrolledExpanded] = useState(false);
	return (
		<WorkingBlockDisclosure
			block={MockWorkingBlock}
			{...props}
			expanded={props.expanded ?? uncontrolledExpanded}
			onExpandedChange={(next) => {
				setUncontrolledExpanded(next);
				props.onExpandedChange?.(next);
			}}
		>
			<ul aria-label="Steps">
				<li>Read src/main.ts</li>
			</ul>
		</WorkingBlockDisclosure>
	);
};

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

	it("advances the live label with the clock", () => {
		vi.useFakeTimers();
		vi.setSystemTime(FIXTURE_NOW);

		try {
			render(
				<ControlledDisclosure
					block={{ ...MockWorkingBlock, isLive: true, endedAt: undefined }}
				/>,
			);
			screen.getByRole("button", { name: "Working for 12s" });

			act(() => {
				vi.advanceTimersByTime(1000);
			});
			screen.getByRole("button", { name: "Working for 13s" });
		} finally {
			vi.useRealTimers();
		}
	});
});

type BlockRowProps = {
	expandedHeight: number;
	defaultExpanded?: boolean;
};

const BlockRow: React.FC<BlockRowProps> = ({
	expandedHeight,
	defaultExpanded = false,
}) => {
	const [expanded, setExpanded] = useState(defaultExpanded);
	return (
		<MessageScroller.Item
			messageId="block"
			data-height={expanded ? expandedHeight : 40}
		>
			<ControlledDisclosure
				expanded={expanded}
				onExpandedChange={setExpanded}
			/>
		</MessageScroller.Item>
	);
};

const summaryButton = () =>
	screen.getByRole("button", { name: "Worked for 12s (2 steps)" });

describe("WorkingBlockDisclosure in a MessageScroller", () => {
	setupMessageScrollerLayout();

	it("scrolls to the end of a block that expands past the view", async () => {
		const user = userEvent.setup();
		renderTranscript([
			history,
			<BlockRow key="block" expandedHeight={300} />,
			row("answer", 100),
		]);

		await user.click(summaryButton());

		expect(await visibleRowsAfterLayout()).toEqual(["history-9", "block"]);
	});

	it("stays put when an expanded block still ends in view", async () => {
		const user = userEvent.setup();
		renderTranscript([
			history,
			<BlockRow key="block" expandedHeight={200} />,
			Array.from({ length: 5 }, (_, index) => row(`after-${index}`, 100)),
		]);
		fireEvent.wheel(viewport());
		viewport().scrollTop = 900;
		fireEvent.scroll(viewport());

		await user.click(summaryButton());

		expect(await visibleRowsAfterLayout()).toEqual([
			"history-9",
			"block",
			"after-0",
		]);
	});

	it("follows new output after expanding the last block of an anchored turn", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		const blockRow = <BlockRow key="block" expandedHeight={600} />;
		await update([history, prompt, blockRow]);

		await user.click(summaryButton());
		await update([history, prompt, blockRow, nextRow]);

		expect(await visibleRowsAfterLayout()).toContain("next");
	});

	it("follows new output after the last block of an anchored turn expands in view", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		const blockRow = <BlockRow key="block" expandedHeight={200} />;
		await update([history, prompt, blockRow]);
		const before = await visibleRowsAfterLayout();

		await user.click(summaryButton());
		expect(await visibleRowsAfterLayout()).toEqual(before);
		// The turn grows past the tail spacer but stays in the handoff band.
		const reply = row("reply", 140);
		await update([history, prompt, blockRow, reply]);
		await flushLayout();
		await update([history, prompt, blockRow, reply, nextRow]);

		expect(await visibleRowsAfterLayout()).toContain("next");
	});

	it("keeps the reader in place when the last block collapses", async () => {
		const user = userEvent.setup();
		const blockRow = (
			<BlockRow key="block" expandedHeight={600} defaultExpanded />
		);
		const update = renderTranscript([history, blockRow]);

		await user.click(summaryButton());
		await update([history, blockRow, nextRow]);

		expect(await visibleRowsAfterLayout()).not.toContain("next");
	});
});

describe("formatLiveElapsed", () => {
	it.each([
		[0, "0s"],
		[12_000, "12s"],
		[60_000, "1m 00s"],
		[66_000, "1m 06s"],
		[1_106_000, "18m 26s"],
		[3_600_000, "1h 00m"],
		[3_900_000, "1h 05m"],
		[-5_000, "0s"],
	])("formats %dms as %s", (ms, expected) => {
		expect(formatLiveElapsed(ms)).toBe(expected);
	});
});

describe("didPrependIntoBlock", () => {
	it.each([
		["older members joining the front", [3, 5], [1, 3, 5], true],
		[
			"a merged first row growing under a stable key",
			[7, 9],
			[4, 5, 7, 9],
			true,
		],
		["unchanged members", [3, 5], [3, 5], false],
		["appended members", [3, 5], [3, 5, 7], false],
		["replaced members", [3, 5], [1, 2], false],
		["the live row becoming its persisted step", [], [7], false],
	])("%s", (_name, previous, next, expected) => {
		expect(didPrependIntoBlock(previous, next)).toBe(expected);
	});
});
