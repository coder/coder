import { render, waitFor } from "@testing-library/react";
import type { Line } from "#/components/Logs/LogLine";
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

const getInnerSizer = (container: HTMLElement): HTMLElement => {
	const outer = container.querySelector<HTMLElement>(
		'div[style*="overflow: auto"]',
	);
	const inner = outer?.firstElementChild as HTMLElement | null;
	if (!inner) {
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
