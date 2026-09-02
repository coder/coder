import { act, render, screen } from "@testing-library/react";
import {
	type ResizeObserverMock,
	setupResizeObserverMock,
} from "#/testHelpers/resizeObserver";
import { useKebabMenu } from "./useKebabMenu";

let resizeObserver: ResizeObserverMock;

const setElementOffsetWidth = (element: HTMLElement, width: number): void => {
	Object.defineProperty(element, "offsetWidth", {
		configurable: true,
		get: () => width,
	});
};

const tabs = [
	{ value: "all", label: "All Logs" },
	{ value: "build", label: "Build Logs" },
	{ value: "startup", label: "Startup Script" },
] as const;

const TestHarness = ({ tabGap = 0 }: { tabGap?: number }) => {
	const { containerRef, visibleTabs, overflowTabs, getTabMeasureProps } =
		useKebabMenu({
			tabs,
			enabled: true,
			isActive: true,
			overflowTriggerWidth: 44,
		});

	return (
		<div>
			<div
				ref={containerRef}
				style={{ display: "flex", columnGap: `${tabGap}px` }}
			>
				{tabs.map((tab) => (
					<button
						key={tab.value}
						type="button"
						{...getTabMeasureProps(tab.value)}
					>
						{tab.label}
					</button>
				))}
			</div>
			<div data-testid="visible-values">
				{visibleTabs.map((tab) => tab.value).join(",")}
			</div>
			<div data-testid="overflow-values">
				{overflowTabs.map((tab) => tab.value).join(",")}
			</div>
		</div>
	);
};

describe("useKebabMenu", () => {
	beforeEach(() => {
		resizeObserver = setupResizeObserverMock();
	});

	afterEach(() => {
		// Keep tests isolated when other suites spy on globals.
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	it("shows all tabs when the available width is enough", async () => {
		render(<TestHarness />);

		const [all, build, startup] = screen.getAllByRole("button");
		setElementOffsetWidth(all, 60);
		setElementOffsetWidth(build, 70);
		setElementOffsetWidth(startup, 70);

		await act(() => {
			resizeObserver.getLast().simulateResize(220);
		});

		expect(screen.getByTestId("visible-values")).toHaveTextContent(
			"all,build,startup",
		);
		expect(screen.getByTestId("overflow-values")).toBeEmptyDOMElement();
	});

	it("accounts for outsideBox tab gap when reserving kebab space", async () => {
		render(<TestHarness tabGap={24} />);

		const [all, build, startup] = screen.getAllByRole("button");
		setElementOffsetWidth(all, 60);
		setElementOffsetWidth(build, 70);
		setElementOffsetWidth(startup, 70);

		await act(() => {
			resizeObserver.getLast().simulateResize(220);
		});

		expect(screen.getByTestId("visible-values")).toHaveTextContent("all");
		expect(screen.getByTestId("overflow-values")).toHaveTextContent(
			"build,startup",
		);
	});
});
