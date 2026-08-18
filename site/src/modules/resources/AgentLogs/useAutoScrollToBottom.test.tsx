import { act, render } from "@testing-library/react";
import { type FC, useRef } from "react";
import type { Line } from "#/components/Logs/LogLine";
import { AgentLogs } from "./AgentLogs";
import { MockSources } from "./mocks";
import { useAutoScrollToBottom } from "./useAutoScrollToBottom";

const VIEWPORT_HEIGHT = 200;
// Mirrors the vertical padding AgentLogs applies to its scroll container
// (py-4 top plus pb-10 bottom when logs overflow). react-window sizes its
// scroll area to itemCount * itemSize and is unaware of this padding, which is
// what made scrollToItem stop short of the bottom (coder/coder#25692).
const CONTAINER_PADDING = 56;

const makeLogs = (count: number): Line[] =>
	Array.from({ length: count }, (_, i) => ({
		id: i + 1,
		level: "info",
		output: `log line ${i + 1}`,
		sourceId: MockSources[0].id,
		time: "2024-03-14T11:31:04.090715Z",
	}));

type HarnessProps = {
	logs: Line[];
	enabled: boolean;
};

// Wires up AgentLogs and the hook the same way the real callers do.
const Harness: FC<HarnessProps> = ({ logs, enabled }) => {
	const outerRef = useRef<HTMLDivElement>(null);
	useAutoScrollToBottom(outerRef, enabled, [logs]);
	return (
		<AgentLogs
			outerRef={outerRef}
			logs={logs}
			sources={MockSources}
			overflowed={false}
			showSourceIcons={false}
			height={VIEWPORT_HEIGHT}
			width={600}
		/>
	);
};

const getScrollContainer = (container: HTMLElement): HTMLElement => {
	const el = container.querySelector<HTMLElement>(
		'div[style*="overflow: auto"]',
	);
	if (!el) {
		throw new Error("react-window scroll container not found");
	}
	return el;
};

// jsdom performs no layout, so emulate the browser's scroll metrics. The
// scrollHeight is derived from the inner sizing element react-window renders
// (itemCount * itemSize) plus the container padding, and scrollTop is clamped
// to the real maximum, exactly like a browser.
const mockScrollMetrics = (outer: HTMLElement): void => {
	let scrollTop = 0;
	Object.defineProperty(outer, "clientHeight", {
		configurable: true,
		get: () => VIEWPORT_HEIGHT,
	});
	Object.defineProperty(outer, "scrollHeight", {
		configurable: true,
		get: () => {
			const inner = outer.firstElementChild as HTMLElement | null;
			const innerHeight = inner
				? Number.parseFloat(inner.style.height || "0")
				: 0;
			return innerHeight + CONTAINER_PADDING;
		},
	});
	Object.defineProperty(outer, "scrollTop", {
		configurable: true,
		get: () => scrollTop,
		set: (value: number) => {
			const max = Math.max(0, outer.scrollHeight - outer.clientHeight);
			scrollTop = Math.min(Math.max(0, value), max);
		},
	});
};

describe("useAutoScrollToBottom", () => {
	it("reaches the true bottom when new logs stream in", () => {
		const { container, rerender } = render(
			<Harness logs={makeLogs(100)} enabled />,
		);
		const outer = getScrollContainer(container);
		mockScrollMetrics(outer);

		act(() => {
			rerender(<Harness logs={makeLogs(101)} enabled />);
		});

		// The last line must be reachable: the visible window has to extend all
		// the way to the bottom of the scroll area (padding included). react-
		// window's scrollToItem would have stopped short by the padding amount.
		expect(outer.scrollTop + outer.clientHeight).toBeGreaterThanOrEqual(
			outer.scrollHeight,
		);
	});

	it("does not scroll when the user has scrolled away", () => {
		const { container, rerender } = render(
			<Harness logs={makeLogs(100)} enabled={false} />,
		);
		const outer = getScrollContainer(container);
		mockScrollMetrics(outer);
		outer.scrollTop = 300;

		act(() => {
			rerender(<Harness logs={makeLogs(101)} enabled={false} />);
		});

		expect(outer.scrollTop).toBe(300);
	});
});
