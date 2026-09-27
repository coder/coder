import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, useState } from "react";
import { FIXTURE_NOW, MockWorkingBlock } from "./storyFixtures";
import {
	didPrependIntoBlock,
	formatWorkingDuration,
	WorkingBlockDisclosure,
} from "./WorkingBlockDisclosure";

const ControlledDisclosure = (
	props: Partial<ComponentProps<typeof WorkingBlockDisclosure>>,
) => {
	const [expanded, setExpanded] = useState(props.expanded ?? false);
	return (
		<WorkingBlockDisclosure
			block={MockWorkingBlock}
			{...props}
			expanded={expanded}
			onExpandedChange={(next) => {
				setExpanded(next);
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

describe("formatWorkingDuration", () => {
	it.each([
		[0, "0s"],
		[999, "0s"],
		[12_000, "12s"],
		[60_000, "1m 0s"],
		[134_000, "2m 14s"],
		[3_600_000, "1h 0m"],
		[3_780_000, "1h 3m"],
		[-5000, "0s"],
	])("formats %d ms as %s", (milliseconds, expected) => {
		expect(formatWorkingDuration(milliseconds)).toBe(expected);
	});
});
