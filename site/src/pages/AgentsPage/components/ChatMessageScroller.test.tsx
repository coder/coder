import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MessageScroller } from "#/vendor/message-scroller";
import { ChatMessageScroller } from "./ChatMessageScroller";

// jsdom does no layout, so give the messages viewport a height and content
// that overflows it.
const isViewport = (element: Element) =>
	element.getAttribute("aria-label") === "Messages";

beforeEach(() => {
	vi.spyOn(Element.prototype, "clientHeight", "get").mockImplementation(
		function (this: Element) {
			return isViewport(this) ? 100 : 0;
		},
	);
	vi.spyOn(Element.prototype, "scrollHeight", "get").mockImplementation(
		function (this: Element) {
			return isViewport(this) ? 1000 : 0;
		},
	);
	// jsdom runs a frame's callbacks in one task, so EarlierMessages' older-page
	// request fires before the re-render that cancels it in a browser.
	vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
		window.setTimeout(() => callback(performance.now())),
	);
	vi.stubGlobal("cancelAnimationFrame", (handle: number) =>
		window.clearTimeout(handle),
	);
	// jsdom has no Element.scrollTo. The scroller only calls its options form.
	Element.prototype.scrollTo = function (
		this: Element,
		options?: ScrollToOptions | number,
	) {
		if (typeof options === "object") {
			this.scrollTop = options.top ?? 0;
		}
	};
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
	Reflect.deleteProperty(Element.prototype, "scrollTo");
});

const nextFrames = () =>
	act(
		() =>
			new Promise<void>((resolve) => {
				requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
			}),
	);

// Without the viewportRef guard only autoScroll: false fails, because with
// autoScroll the scroller follows the end once the viewport attaches.
it.each([true, false])(
	"opens at the end without loading older messages when a transcript mounts after its scroller provider (autoScroll: %s)",
	async (autoScroll) => {
		const onFetchMoreMessages = vi.fn();
		const page = (transcript?: React.ReactNode) => (
			<MessageScroller.Provider
				autoScroll={autoScroll}
				defaultScrollPosition="end"
			>
				{transcript}
			</MessageScroller.Provider>
		);

		// The provider mounts while the page shows its loading view.
		const { rerender, getByLabelText } = render(page());
		rerender(
			page(
				<ChatMessageScroller
					hasMoreMessages
					isFetchingMoreMessages={false}
					hasFetchMoreError={false}
					hasTranscriptRows
					onFetchMoreMessages={onFetchMoreMessages}
				>
					{["1", "2", "3"].map((id) => (
						<MessageScroller.Item key={id} messageId={id}>
							message {id}
						</MessageScroller.Item>
					))}
				</ChatMessageScroller>,
			),
		);
		await nextFrames();

		const viewport = getByLabelText("Messages");
		expect(viewport.scrollTop).toBe(
			viewport.scrollHeight - viewport.clientHeight,
		);
		expect(onFetchMoreMessages).not.toHaveBeenCalled();
	},
);
