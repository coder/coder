import { act, render, screen } from "@testing-library/react";
import { useKebabMenu } from "./useKebabMenu";

type FakeResizeObserverInstance = {
	simulateResize: (width: number) => void;
};

let resizeObserverInstances: FakeResizeObserverInstance[] = [];

class MockResizeObserver {
	private readonly callback: ResizeObserverCallback;

	constructor(callback: ResizeObserverCallback) {
		this.callback = callback;
		const self = this;
		resizeObserverInstances.push({
			simulateResize(width: number) {
				self.callback(
					[{ contentRect: { width, height: 0 } } as ResizeObserverEntry],
					self as unknown as ResizeObserver,
				);
			},
		});
	}

	observe(_target: Element) {}
	unobserve(_target: Element) {}
	disconnect() {}
}

const getLastResizeObserver = (): FakeResizeObserverInstance => {
	const instance = resizeObserverInstances[resizeObserverInstances.length - 1];
	if (!instance) {
		throw new Error("No ResizeObserver was constructed");
	}
	return instance;
};

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
		resizeObserverInstances = [];
		vi.stubGlobal("ResizeObserver", MockResizeObserver);
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
			getLastResizeObserver().simulateResize(220);
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
			getLastResizeObserver().simulateResize(220);
		});

		expect(screen.getByTestId("visible-values")).toHaveTextContent("all");
		expect(screen.getByTestId("overflow-values")).toHaveTextContent(
			"build,startup",
		);
	});
});
