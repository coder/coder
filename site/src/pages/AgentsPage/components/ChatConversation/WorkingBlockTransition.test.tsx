import { act, render, screen } from "@testing-library/react";
import * as motionReact from "motion/react";
import {
	afterAll,
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { WorkingBlockTransition } from "./WorkingBlockTransition";

// Motion keeps the requestAnimationFrame it finds when it loads, so route it
// through setTimeout first; then fake timers step Motion's frames.
vi.hoisted(() => {
	vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
		setTimeout(() => callback(performance.now()), 16),
	);
});

vi.mock("motion/react", async (importOriginal) => ({
	...(await importOriginal<typeof motionReact>()),
	useReducedMotion: vi.fn(() => false),
}));

// Motion reports completion through a promise, so timers advance async.
const advance = (ms: number) =>
	act(async () => {
		await vi.advanceTimersByTimeAsync(ms);
	});

type Props = React.ComponentProps<typeof WorkingBlockTransition>;

const fold = (id: number) => ({ id, content: `Reply ${id}` });

const renderTransition = (props: Partial<Props>) => {
	const onLabelEntered = vi.fn();
	const onFolded = vi.fn();
	const view = (next: Partial<Props>) => (
		<WorkingBlockTransition
			labelEntering={false}
			folds={[]}
			isLive
			onLabelEntered={onLabelEntered}
			onFolded={onFolded}
			{...next}
		>
			Working
		</WorkingBlockTransition>
	);
	const { rerender } = render(view(props));
	return {
		onLabelEntered,
		onFolded,
		rerender: (next: Partial<Props>) => rerender(view(next)),
	};
};

describe("WorkingBlockTransition", () => {
	// One fake clock for the whole block: Motion's frame loop keeps waiting on
	// a frame scheduled on a clock that a later test replaced.
	beforeAll(() => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "performance"] });
	});
	afterAll(() => {
		vi.useRealTimers();
	});
	beforeEach(() => {
		vi.mocked(motionReact.useReducedMotion).mockReturnValue(false);
	});
	afterEach(async () => {
		await advance(1000);
	});

	it("enters the label, pauses, then folds the reply", async () => {
		const { onLabelEntered, onFolded, rerender } = renderTransition({
			labelEntering: true,
			folds: [fold(1)],
		});

		await advance(280);
		expect(onLabelEntered).not.toHaveBeenCalled();
		expect(screen.getByRole("status")).toHaveTextContent("");

		await advance(40);
		expect(onLabelEntered).toHaveBeenCalledOnce();
		expect(screen.getByRole("status")).toHaveTextContent("Working");
		rerender({ labelEntering: false, folds: [fold(1)] });

		// The 60ms pause, then the 180ms fade, which ends the fold in jsdom:
		// without layout the height step has nothing to animate.
		await advance(220);
		expect(onFolded).not.toHaveBeenCalled();
		await advance(40);
		expect(onFolded).toHaveBeenCalledWith(1);
	});

	it("folds into a label that is already showing without a pause", async () => {
		const { onLabelEntered, onFolded } = renderTransition({
			folds: [fold(1)],
		});

		await advance(160);
		expect(onFolded).not.toHaveBeenCalled();
		await advance(40);
		expect(onFolded).toHaveBeenCalledWith(1);
		expect(onLabelEntered).not.toHaveBeenCalled();
	});

	it("folds queued replies one at a time", async () => {
		const { onFolded, rerender } = renderTransition({
			folds: [fold(1), fold(2)],
		});

		await advance(200);
		expect(onFolded.mock.calls).toEqual([[1]]);
		rerender({ folds: [fold(2)] });

		await advance(200);
		expect(onFolded.mock.calls).toEqual([[1], [2]]);
	});

	it("crossfades both beats together under reduced motion", async () => {
		vi.mocked(motionReact.useReducedMotion).mockReturnValue(true);
		const { onLabelEntered, onFolded } = renderTransition({
			labelEntering: true,
			folds: [fold(1)],
		});

		await advance(170);
		expect(onLabelEntered).toHaveBeenCalledOnce();
		expect(onFolded).toHaveBeenCalledWith(1);
	});
});
