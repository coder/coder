import { render, screen } from "@testing-library/react";
import { WorkingBlockDisclosure } from "./WorkingBlockDisclosure";
import type { WorkingBlock } from "./workingBlockGrouping";

const block: WorkingBlock = {
	key: "working:through:message:5",
	liveKey: "working:live:message:1:0",
	rowIndices: [0, 1],
	startedAt: 0,
	endedAt: 12_000,
	stepCount: 2,
	failedCount: 0,
	isLive: false,
	isPartial: false,
};

const renderInScroller = (rowKeys: readonly string[]) => (
	<div role="region" aria-label="Messages" style={{ overflowY: "auto" }}>
		<WorkingBlockDisclosure
			block={block}
			rowKeys={rowKeys}
			expanded
			onExpandedChange={() => {}}
			now={12_000}
		>
			<p>steps</p>
		</WorkingBlockDisclosure>
	</div>
);

describe("WorkingBlockDisclosure", () => {
	let contentHeight = 0;

	beforeEach(() => {
		contentHeight = 100;
		vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(
			() => contentHeight,
		);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("scrolls by the growth when older rows join the front", () => {
		const { rerender } = render(renderInScroller(["message:3", "message:5"]));
		const viewport = screen.getByRole("region", { name: "Messages" });
		viewport.scrollTop = 400;

		contentHeight = 160;
		rerender(renderInScroller(["message:1", "message:3", "message:5"]));

		expect(viewport.scrollTop).toBe(460);
	});

	it("holds the scroll when the live row becomes its persisted step", () => {
		const { rerender } = render(renderInScroller(["live:1"]));
		const viewport = screen.getByRole("region", { name: "Messages" });
		viewport.scrollTop = 400;

		contentHeight = 160;
		rerender(renderInScroller(["message:7"]));

		expect(viewport.scrollTop).toBe(400);
	});
});
