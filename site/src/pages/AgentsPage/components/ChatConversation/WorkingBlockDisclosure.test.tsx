import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
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
import { MessageScroller, useMessageScroller } from "#/vendor/message-scroller";
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
	width?: number;
	/** A live block, collapsed to its summary and a peek at its last step. */
	live?: boolean;
};

const BlockRow: React.FC<BlockRowProps> = ({
	expandedHeight,
	defaultExpanded = false,
	width,
	live = false,
}) => {
	const [expanded, setExpanded] = useState(defaultExpanded);
	return (
		<MessageScroller.Item
			messageId="block"
			data-height={expanded ? expandedHeight : 40}
			data-width={width}
		>
			<ControlledDisclosure
				expanded={expanded}
				onExpandedChange={setExpanded}
				{...(live && {
					block: { ...MockWorkingBlock, isLive: true, endedAt: undefined },
					peek: {
						items: [
							{
								kind: "thinking",
								key: "thinking:start:0",
								text: "**Reading the config**\n\nCheck the defaults first.",
								isStreaming: false,
							},
						],
						organizationId: "organization-id",
						mcpServers: [],
						subagentTitles: new Map(),
						subagentVariants: new Map(),
					},
				})}
			/>
		</MessageScroller.Item>
	);
};

const summaryButton = () =>
	screen.getByRole("button", { name: "Worked for 12s (2 steps)" });

const JumpRow: React.FC = () => {
	const { scrollToMessage } = useMessageScroller();
	return (
		<MessageScroller.Item messageId="jump" data-height={0}>
			<button type="button" onClick={() => scrollToMessage("history-9")}>
				Jump to history-9
			</button>
		</MessageScroller.Item>
	);
};

type RevealTranscriptProps = {
	expandedHeight: number;
	width?: number;
};

const RevealTranscript: React.FC<RevealTranscriptProps> = ({
	expandedHeight,
	width,
}) => (
	<>
		{history}
		<BlockRow expandedHeight={expandedHeight} width={width} />
		{row("answer", 100)}
		<JumpRow />
	</>
);

describe("WorkingBlockDisclosure in a MessageScroller", () => {
	setupMessageScrollerLayout();

	it.each([
		["its summary", false, () => summaryButton()],
		// The peek unmounts as the block expands.
		["its peek", true, () => screen.getByText(/reading the config/i)],
	])(
		"scrolls to the end of a block that expands past the view from %s",
		async (_name, live, target) => {
			const user = userEvent.setup();
			renderTranscript([
				history,
				<BlockRow key="block" expandedHeight={300} live={live} />,
				row("answer", 100),
			]);

			await user.click(target());

			expect(await visibleRowsAfterLayout()).toEqual(["history-9", "block"]);
		},
	);

	it("scrolls a block taller than the view only until its summary reaches the top", async () => {
		const user = userEvent.setup();
		const update = renderTranscript(<RevealTranscript expandedHeight={600} />);

		await user.click(summaryButton());
		await flushLayout();
		expect(viewport().scrollTop).toBe(1000);

		// Find in page scrolls the view into the block, and then a step grows.
		viewport().scrollTop = 1100;
		fireEvent.scroll(viewport());
		await update(<RevealTranscript expandedHeight={700} />);
		await flushLayout();
		expect(viewport().scrollTop).toBe(1100);
	});

	it.each([
		["glides", "no-preference", "smooth"],
		["jumps", "reduce", "auto"],
	] as const)(
		"%s to the end of an expanded block when the reader's motion preference is %s",
		async (_name, preference, behavior) => {
			vi.stubGlobal("matchMedia", (query: string) => ({
				matches: query === `(prefers-reduced-motion: ${preference})`,
			}));
			const scrollTo = vi.spyOn(Element.prototype, "scrollTo");
			const user = userEvent.setup();
			renderTranscript(<RevealTranscript expandedHeight={300} />);

			await user.click(summaryButton());

			expect(scrollTo).toHaveBeenLastCalledWith(
				expect.objectContaining({ behavior }),
			);
		},
	);

	it("keeps the end of a revealed block in view as its steps finish loading", async () => {
		const user = userEvent.setup();
		const update = renderTranscript(<RevealTranscript expandedHeight={300} />);
		await user.click(summaryButton());
		await flushLayout();

		// Lazy images in two of the steps load, one after the other.
		await update(<RevealTranscript expandedHeight={320} />);
		await flushLayout();
		await update(<RevealTranscript expandedHeight={380} />);

		expect(await visibleRowsAfterLayout()).toEqual(["block"]);
	});

	it.each([
		[
			"the reader presses a scroll key",
			(user: UserEvent) => user.keyboard("{PageUp}"),
		],
		[
			"the view scrolls up without a gesture",
			() => {
				viewport().scrollTop = 800;
				fireEvent.scroll(viewport());
			},
		],
		[
			"the reader jumps to another row",
			(user: UserEvent) =>
				user.click(screen.getByRole("button", { name: "Jump to history-9" })),
		],
		[
			"a while passes",
			() => {
				const now = performance.now.bind(performance);
				vi.spyOn(performance, "now").mockImplementation(() => now() + 60_000);
			},
		],
		[
			"the block re-wraps at a new width",
			(_user: UserEvent, update: ReturnType<typeof renderTranscript>) =>
				update(<RevealTranscript expandedHeight={300} width={600} />),
		],
	])("stops revealing a block once %s", async (_name, intervene) => {
		const user = userEvent.setup();
		const update = renderTranscript(<RevealTranscript expandedHeight={300} />);
		await user.click(summaryButton());
		await flushLayout();
		await intervene(user, update);
		const before = await visibleRowsAfterLayout();

		await update(<RevealTranscript expandedHeight={500} />);

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("does not scroll back up to the end of a revealed block that grows in view", async () => {
		const user = userEvent.setup();
		const update = renderTranscript(<RevealTranscript expandedHeight={300} />);
		await user.click(summaryButton());
		await flushLayout();
		// Find in page scrolls the view down, leaving the block's end in view.
		viewport().scrollTop = 1000;
		fireEvent.scroll(viewport());
		const before = await visibleRowsAfterLayout();

		await update(<RevealTranscript expandedHeight={350} />);

		expect(await visibleRowsAfterLayout()).toEqual(before);
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

	it.each([
		["", [], 140],
		[" above a later row of its turn", [row("narration", 40)], 100],
	])(
		"follows new output after the last block of an anchored turn expands in view%s",
		async (_name, rowsBelow, replyHeight) => {
			const user = userEvent.setup();
			const update = renderTranscript([history]);
			const blockRow = <BlockRow key="block" expandedHeight={200} />;
			await update([history, prompt, blockRow, rowsBelow]);
			const before = await visibleRowsAfterLayout();

			await user.click(summaryButton());
			expect(await visibleRowsAfterLayout()).toEqual(before);
			// The turn grows past the tail spacer but stays in the handoff band.
			const reply = row("reply", replyHeight);
			await update([history, prompt, blockRow, rowsBelow, reply]);
			await flushLayout();
			await update([history, prompt, blockRow, rowsBelow, reply, nextRow]);

			expect(await visibleRowsAfterLayout()).toContain("next");
		},
	);

	it("follows new output after the last block of an anchored turn expands to fill the view", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		// Ends at the bottom of the view, using up the tail spacer.
		const blockRow = <BlockRow key="block" expandedHeight={296} />;
		await update([history, prompt, blockRow]);

		await user.click(summaryButton());
		await update([history, prompt, blockRow, nextRow]);

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
