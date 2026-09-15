import { fireEvent, render, waitFor } from "@testing-library/react";
import type { Line } from "#/components/Logs/LogLine";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { AGENT_LOG_LINE_HEIGHT } from "./AgentLogLine";
import { AgentLogs } from "./AgentLogs";
import { MockSources } from "./mocks";

// A real log row renders taller than the AGENT_LOG_LINE_HEIGHT estimate (e.g.
// when output wraps). We simulate that by reporting a fixed measured height
// that is larger than the estimate.
const MEASURED_ROW_HEIGHT = 40;
const ROW_COUNT = 5;

const makeLogs = (count: number): Line[] =>
	Array.from({ length: count }, (_, i) => ({
		id: i + 1,
		level: "info",
		output: `log line ${i + 1}`,
		sourceId: MockSources[0].id,
		time: "2024-03-14T11:31:04.090715Z",
	}));

const getScrollContainer = (container: HTMLElement): HTMLElement => {
	const outer = container.querySelector<HTMLElement>(
		'div[style*="overflow: auto"]',
	);
	if (!outer) {
		throw new Error("react-window scroll container not found");
	}
	return outer;
};

const getInnerSizer = (container: HTMLElement): HTMLElement => {
	const inner = getScrollContainer(container).firstElementChild;
	if (!(inner instanceof HTMLElement)) {
		throw new Error("react-window inner sizing element not found");
	}
	return inner;
};

describe("AgentLogs virtualized height", () => {
	let boundingRect: ReturnType<typeof vi.spyOn>;

	beforeAll(() => {
		// jsdom performs no layout, so make every row report a height that is
		// taller than the fixed estimate. The list must measure and use this
		// height rather than assuming AGENT_LOG_LINE_HEIGHT per row.
		boundingRect = vi
			.spyOn(HTMLElement.prototype, "getBoundingClientRect")
			.mockReturnValue({
				height: MEASURED_ROW_HEIGHT,
				width: 600,
				top: 0,
				left: 0,
				right: 600,
				bottom: MEASURED_ROW_HEIGHT,
				x: 0,
				y: 0,
				toJSON: () => ({}),
			} as DOMRect);
	});

	afterAll(() => {
		boundingRect.mockRestore();
	});

	it("sizes the scroll area from measured row heights, not the fixed estimate", async () => {
		const { container } = render(
			<AgentLogs
				logs={makeLogs(ROW_COUNT)}
				sources={MockSources}
				overflowed={false}
				showSourceIcons={false}
				// Tall enough to render (and therefore measure) every row.
				height={1000}
				width={600}
			/>,
		);

		// The virtualized height must account for the real, taller rows.
		// react-window's fixed itemSize would produce ROW_COUNT *
		// AGENT_LOG_LINE_HEIGHT, which is short of the real content and is what
		// left the last lines unreachable (coder/coder#25692).
		await waitFor(() => {
			const totalHeight = Number.parseFloat(
				getInnerSizer(container).style.height,
			);
			expect(totalHeight).toBe(ROW_COUNT * MEASURED_ROW_HEIGHT);
		});
		expect(
			Number.parseFloat(getInnerSizer(container).style.height),
		).toBeGreaterThan(ROW_COUNT * AGENT_LOG_LINE_HEIGHT);
	});
});

describe("AgentLogs follow", () => {
	const CLIENT_HEIGHT = 256;
	const INITIAL_SCROLL_HEIGHT = 1000;
	const GROWN_SCROLL_HEIGHT = 2000;
	let scrollHeight = INITIAL_SCROLL_HEIGHT;

	const baseProps = {
		logs: makeLogs(50),
		sources: MockSources,
		overflowed: false,
		showSourceIcons: false,
		height: CLIENT_HEIGHT,
		width: 600,
	};

	beforeEach(() => {
		scrollHeight = INITIAL_SCROLL_HEIGHT;
		MockResizeObserver.reset();
		vi.stubGlobal("ResizeObserver", MockResizeObserver);
		// jsdom performs no layout
		vi.spyOn(Element.prototype, "scrollHeight", "get").mockImplementation(
			() => scrollHeight,
		);
		vi.spyOn(Element.prototype, "clientHeight", "get").mockReturnValue(
			CLIENT_HEIGHT,
		);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	const getContentObserver = (inner: HTMLElement): MockResizeObserver => {
		const observer = MockResizeObserver.instances.find((o) =>
			o.observe.mock.calls.some(([target]) => target === inner),
		);
		if (!observer) {
			throw new Error("no ResizeObserver observes the inner sizing element");
		}
		return observer;
	};

	it("stops following when the user scrolls up", () => {
		const onFollowChange = vi.fn();
		const { container } = render(
			<AgentLogs {...baseProps} follow onFollowChange={onFollowChange} />,
		);
		const outer = getScrollContainer(container);
		expect(outer.scrollTop).toBe(INITIAL_SCROLL_HEIGHT);

		outer.scrollTop = 500;
		fireEvent.scroll(outer);

		expect(onFollowChange).toHaveBeenCalledWith(false);
	});

	it("resumes following when the user scrolls back to the bottom", () => {
		const onFollowChange = vi.fn();
		const { container } = render(
			<AgentLogs
				{...baseProps}
				follow={false}
				onFollowChange={onFollowChange}
			/>,
		);
		const outer = getScrollContainer(container);

		outer.scrollTop = 500;
		fireEvent.scroll(outer);
		expect(onFollowChange).not.toHaveBeenCalled();

		outer.scrollTop = INITIAL_SCROLL_HEIGHT - CLIENT_HEIGHT;
		fireEvent.scroll(outer);
		expect(onFollowChange).toHaveBeenCalledWith(true);
	});

	it("keeps the newest line in view when content grows while following", () => {
		const { container } = render(
			<AgentLogs {...baseProps} follow onFollowChange={vi.fn()} />,
		);
		const outer = getScrollContainer(container);
		const observer = getContentObserver(getInnerSizer(container));

		scrollHeight = GROWN_SCROLL_HEIGHT;
		observer.simulateResize(600, GROWN_SCROLL_HEIGHT);

		expect(outer.scrollTop).toBe(GROWN_SCROLL_HEIGHT);
	});

	it("leaves the scroll position alone when content grows while not following", () => {
		const { container } = render(
			<AgentLogs {...baseProps} follow={false} onFollowChange={vi.fn()} />,
		);
		const outer = getScrollContainer(container);
		const observer = getContentObserver(getInnerSizer(container));
		outer.scrollTop = 300;

		scrollHeight = GROWN_SCROLL_HEIGHT;
		observer.simulateResize(600, GROWN_SCROLL_HEIGHT);

		expect(outer.scrollTop).toBe(300);
	});

	it("scrolls to the bottom when follow is turned on", () => {
		const onFollowChange = vi.fn();
		const { container, rerender } = render(
			<AgentLogs
				{...baseProps}
				follow={false}
				onFollowChange={onFollowChange}
			/>,
		);
		const outer = getScrollContainer(container);
		outer.scrollTop = 300;

		rerender(
			<AgentLogs {...baseProps} follow onFollowChange={onFollowChange} />,
		);

		expect(outer.scrollTop).toBe(INITIAL_SCROLL_HEIGHT);
	});
});
