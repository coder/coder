import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, useState } from "react";
import { FIXTURE_NOW, MockWorkingBlock } from "./storyFixtures";
import { WorkingBlockDisclosure } from "./WorkingBlockDisclosure";

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
