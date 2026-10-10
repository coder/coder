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
	// jsdom has no Element.scrollTo.
	Element.prototype.scrollTo = function (
		this: Element,
		optionsOrX?: ScrollToOptions | number,
	) {
		this.scrollTop =
			typeof optionsOrX === "number" ? optionsOrX : (optionsOrX?.top ?? 0);
	};
});

afterEach(() => {
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

it.each([true, false])(
	"loads no older messages when a transcript mounts after its scroller provider (autoScroll: %s)",
	async (autoScroll) => {
		const onFetchMoreMessages = vi.fn();
		const page = (transcript: boolean) => (
			<MessageScroller.Provider
				autoScroll={autoScroll}
				defaultScrollPosition="end"
			>
				{transcript && (
					<ChatMessageScroller
						hasMoreMessages
						isFetchingMoreMessages={false}
						isHydratingMessages={false}
						hasFetchMoreError={false}
						hasTranscriptRows
						onFetchMoreMessages={onFetchMoreMessages}
					>
						{["1", "2", "3"].map((id) => (
							<MessageScroller.Item key={id} messageId={id}>
								message {id}
							</MessageScroller.Item>
						))}
					</ChatMessageScroller>
				)}
			</MessageScroller.Provider>
		);

		// The provider mounts while the page shows its loading view.
		const { rerender } = render(page(false));
		rerender(page(true));
		await nextFrames();

		expect(onFetchMoreMessages).not.toHaveBeenCalled();
	},
);
