import { act, renderHook } from "@testing-library/react";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { useChatOverview } from "./useChatOverview";

beforeEach(() => {
	MockResizeObserver.reset();
	vi.stubGlobal("ResizeObserver", MockResizeObserver);
	document.documentElement.style.fontSize = "16px";
});

afterEach(() => {
	vi.unstubAllGlobals();
	document.documentElement.style.fontSize = "";
	localStorage.clear();
});

const renderOverview = (chatWidthPx: number) => {
	const { result } = renderHook(() =>
		useChatOverview({ isRightPanelOpen: false }),
	);
	act(() => result.current.observeChat(document.createElement("div")));
	act(() => MockResizeObserver.getLast().simulateResize(chatWidthPx));
	return result;
};

describe("useChatOverview", () => {
	it("hides the card and its toggle in a chat narrower than 84rem", () => {
		const result = renderOverview(84 * 16 - 1);

		expect(result.current.isCardShown).toBe(false);
		expect(result.current.toggle).toBeUndefined();
	});

	it("shows the card and its toggle from an 84rem chat on", () => {
		const result = renderOverview(84 * 16);

		expect(result.current.isCardShown).toBe(true);
		expect(result.current.toggle?.isOpen).toBe(true);
	});
});
