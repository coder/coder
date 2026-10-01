import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import type { AnnotatorToHostMessage } from "#/annotator/protocol";
import { createDeferred } from "#/testHelpers/deferred";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { portForwardURL } from "#/utils/portForward";
import {
	ComposerContext,
	type ComposerSendResult,
} from "../../context/ComposerContext";
import {
	type TabPopoutMessage,
	tabPopoutChannelName,
} from "../../utils/rightPanelTabPopout";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { PortPreviewPanel } from "./PortPreviewPanel";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const tab: Extract<UserRightPanelTab, { kind: "port" }> = {
	id: "port-3000",
	kind: "port",
	label: "Preview :3000",
	agentId: MockWorkspaceAgent.id,
	port: 3000,
	protocol: "http",
};

type Send = (message: string) => Promise<ComposerSendResult>;

const sent = () => vi.fn<Send>().mockResolvedValue("sent");

function frameWindow(frame: HTMLIFrameElement): Window {
	if (!frame.contentWindow) {
		throw new Error("frame has no window");
	}
	return frame.contentWindow;
}

function renderPanel(onSend = sent(), readyTimeoutMs?: number) {
	const panel = (send: Send | undefined, isAgentWorking: boolean) => (
		<ComposerContext value={send ? { send } : undefined}>
			<PortPreviewPanel
				chatId="chat-1"
				workspace={MockWorkspace}
				agent={MockWorkspaceAgent}
				host="*.apps.example.com"
				tab={tab}
				canAnnotate
				isAgentWorking={isAgentWorking}
				annotatorReadyTimeoutMs={readyTimeoutMs}
			/>
		</ComposerContext>
	);
	const { rerender, unmount } = renderComponent(panel(onSend, false));
	const setAgentWorking = (isAgentWorking: boolean) =>
		rerender(panel(onSend, isAgentWorking));
	// Requesting the overlay remounts the iframe, so always look it up fresh.
	const frame = () => screen.getByTitle<HTMLIFrameElement>("Preview :3000");
	const frameOrigin = new URL(frame().src).origin;
	const receive = (data: AnnotatorToHostMessage) => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data,
				origin: frameOrigin,
				source: frameWindow(frame()),
			}),
		);
	};
	return {
		frame,
		frameOrigin,
		receive,
		onSend,
		setComposer: (send?: Send) => rerender(panel(send, false)),
		setAgentWorking,
		unmount,
	};
}

// The origin the panel loads the preview from; submissions must come
// from a page there.
const previewOrigin = new URL(
	portForwardURL(
		"*.apps.example.com",
		tab.port,
		MockWorkspaceAgent.name,
		MockWorkspace.name,
		MockWorkspace.owner_name,
		tab.protocol,
	),
).origin;

const submission: AnnotatorToHostMessage = {
	type: "coder-annotator:submit",
	page: {
		url: `${previewOrigin}/`,
		title: "App",
		viewport: { width: 800, height: 600 },
	},
	annotations: [
		{
			id: "a",
			comment: "Make this red",
			element: {
				tag: "button",
				selector: "#save",
				classes: [],
				openingTag: '<button id="save">',
				rect: { x: 1, y: 2, width: 3, height: 4 },
			},
		},
	],
};

const annotateButton = () =>
	screen.getByRole("button", { name: /annotate elements|stop annotating/i });

const requestOverlay = () => userEvent.click(annotateButton());

// Sends run on a promise chain, so proving one did not happen needs a
// turn of the event loop rather than a `waitFor`.
const settled = () => new Promise((resolve) => setTimeout(resolve, 20));

describe("PortPreviewPanel annotations", () => {
	it("requests the overlay and starts picking once it is ready", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();

		expect(new URL(frame().src).searchParams.get("coder_annotate")).toBe("1");
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		expect(postMessage).not.toHaveBeenCalled();

		receive({ type: "coder-annotator:ready" });
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
	});

	it("does not reload the frame for clicks while the overlay is loading", async () => {
		const { frame } = renderPanel();
		await requestOverlay();
		const requestedSrc = frame().src;
		const requestedFrame = frame();

		await userEvent.click(annotateButton());
		expect(frame()).toBe(requestedFrame);
		expect(frame().src).toBe(requestedSrc);
	});

	it("lets a click while the overlay is loading cancel the request", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();
		frame().dispatchEvent(new Event("load"));
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");

		// Second click: the user changed their mind before the overlay loaded.
		await userEvent.click(annotateButton());
		receive({ type: "coder-annotator:ready" });
		expect(postMessage).not.toHaveBeenCalled();

		// Asking again after the overlay is ready no longer needs a reload.
		const readyFrame = frame();
		await userEvent.click(annotateButton());
		expect(frame()).toBe(readyFrame);
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
	});

	it("keeps the request through the overlay's initial state reports", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		// The overlay reports idle when it mounts and again after ready, before
		// it has processed the dashboard's request to pick.
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: false });
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: false });
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "true");

		receive(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
	});

	it("moves focus into the preview once the overlay confirms picking", async () => {
		const { frame, receive } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const focus = vi.spyOn(frame(), "focus");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(focus).toHaveBeenCalled();
	});

	it("keeps a new request when the overlay acknowledges the stop before it late", async () => {
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
		});
		// Off and on again before the overlay has echoed the stop.
		await userEvent.click(annotateButton());
		await userEvent.click(annotateButton());
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: false });
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "true");
		expect(postMessage).not.toHaveBeenCalledWith(
			expect.objectContaining({ picking: false }),
			frameOrigin,
		);
		receive(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
	});

	it("sends each submission to the agent as a message", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		const [message] = onSend.mock.calls[0];
		expect(message).toContain("# UI annotations");
		expect(message).toContain("> Make this red");
		expect(message).toContain("`#save`");
	});

	it("ignores submissions claiming a page at another origin", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive({
			...submission,
			page: { ...submission.page, url: "https://elsewhere.example.com/admin" },
		});
		await settled();
		expect(onSend).not.toHaveBeenCalled();
	});

	it("bounds how many submissions wait to be sent", async () => {
		vi.mocked(toast.error).mockClear();
		const first = createDeferred<ComposerSendResult>();
		const onSend = vi
			.fn<Send>()
			.mockReturnValueOnce(first.promise)
			.mockResolvedValue("sent");
		const { receive } = renderPanel(onSend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		// A flood while the first send is still in flight.
		for (let index = 0; index < 6; index++) {
			receive({
				...submission,
				annotations: [{ ...submission.annotations[0], id: `${index}` }],
			});
		}
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(toast.error).toHaveBeenCalledWith(
			"Too many UI annotations at once; the latest was not sent.",
			expect.anything(),
		);

		first.resolve("sent");
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(3));
		await settled();
		expect(onSend).toHaveBeenCalledTimes(3);
	});

	it("drops waiting submissions once annotate mode is turned off", async () => {
		vi.mocked(toast.error).mockClear();
		const first = createDeferred<ComposerSendResult>();
		const onSend = vi
			.fn<Send>()
			.mockReturnValueOnce(first.promise)
			.mockResolvedValue("sent");
		const { receive } = renderPanel(onSend);
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
		});
		for (const id of ["a", "b", "c"]) {
			receive({
				...submission,
				annotations: [{ ...submission.annotations[0], id }],
			});
		}
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));

		// Leaving annotate mode withdraws the page's authorization. The send
		// in flight was accepted while it was on and completes; the rest go.
		await userEvent.keyboard("{Escape}");
		expect(toast.error).toHaveBeenCalledWith(
			"2 UI annotations were not sent: annotate mode was turned off first.",
			expect.anything(),
		);
		first.resolve("sent");
		await settled();
		expect(onSend).toHaveBeenCalledTimes(1);
	});

	it("accepts a submission through the new handler after rerender without rearming", async () => {
		const originalSend = sent();
		const nextSend = sent();
		const { receive, setComposer } = renderPanel(originalSend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });

		setComposer(nextSend);
		receive(submission);
		await waitFor(() => expect(nextSend).toHaveBeenCalledTimes(1));
		expect(originalSend).not.toHaveBeenCalled();
	});

	it("uses the latest committed sender for queued messages after an ordinary rerender", async () => {
		const first = createDeferred<ComposerSendResult>();
		const originalSend = vi.fn<Send>().mockReturnValue(first.promise);
		const nextSend = sent();
		const { receive, setComposer } = renderPanel(originalSend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		await waitFor(() => expect(originalSend).toHaveBeenCalledTimes(1));
		receive({
			...submission,
			annotations: [{ ...submission.annotations[0], id: "b" }],
		});
		setComposer(nextSend);
		first.resolve("sent");
		await waitFor(() => expect(nextSend).toHaveBeenCalledTimes(1));
		expect(originalSend).toHaveBeenCalledTimes(1);
	});

	it("does not call a disabled composer while retrying, then uses the latest sender", async () => {
		const busySend = vi.fn<Send>().mockResolvedValue("busy");
		const nextSend = sent();
		const { receive, setComposer } = renderPanel(busySend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		vi.useFakeTimers();
		try {
			await act(async () => {
				receive(submission);
			});
			expect(busySend).toHaveBeenCalledTimes(1);
			setComposer(undefined);
			await act(async () => {
				await vi.advanceTimersByTimeAsync(500);
			});
			expect(busySend).toHaveBeenCalledTimes(1);
			setComposer(nextSend);
			await act(async () => {
				await vi.advanceTimersByTimeAsync(500);
			});
			expect(nextSend).toHaveBeenCalledTimes(1);
			expect(busySend).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("does not retry through the old composer after unmount", async () => {
		const busySend = vi.fn<Send>().mockResolvedValue("busy");
		const { receive, unmount } = renderPanel(busySend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		vi.useFakeTimers();
		try {
			await act(async () => {
				receive(submission);
			});
			expect(busySend).toHaveBeenCalledTimes(1);
			unmount();
			await act(async () => {
				await vi.advanceTimersByTimeAsync(500);
			});
			expect(busySend).toHaveBeenCalledTimes(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("sends submissions in order and retries while the chat is busy", async () => {
		const onSend = vi
			.fn<Send>()
			.mockResolvedValueOnce("busy")
			.mockResolvedValue("sent");
		const { receive } = renderPanel(onSend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		receive({
			...submission,
			annotations: [
				{ ...submission.annotations[0], id: "b", comment: "Then this" },
			],
		});

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(3), {
			timeout: 3000,
		});
		expect(onSend.mock.calls[0][0]).toContain("> Make this red");
		expect(onSend.mock.calls[1][0]).toContain("> Make this red");
		expect(onSend.mock.calls[2][0]).toContain("> Then this");
	});

	it("stops asking for the first-run hint once it was dismissed", async () => {
		window.localStorage.removeItem("coder.annotator.hint-dismissed");
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive({ type: "coder-annotator:hint-dismissed" });
		expect(window.localStorage.getItem("coder.annotator.hint-dismissed")).toBe(
			"1",
		);

		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: true });
		});
		await userEvent.keyboard("{Escape}");
		expect(postMessage).toHaveBeenLastCalledWith(
			{ type: "coder-annotator:set-picking", picking: false, hint: false },
			frameOrigin,
		);
		window.localStorage.removeItem("coder.annotator.hint-dismissed");
	});

	it("ignores the frame until the user requests the overlay", () => {
		const { receive, onSend } = renderPanel();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("ignores submissions once the user has left annotate mode", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
			// The user switches picking off from inside the preview; anything
			// the page posts afterwards is not an annotation the user made.
			receive({ type: "coder-annotator:state", picking: false });
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive(submission);
		await settled();
		expect(onSend).not.toHaveBeenCalled();
	});

	it("refuses picking the dashboard did not start", async () => {
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
		});
		await userEvent.keyboard("{Escape}");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: false });
		});
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");

		// The page claims annotate mode is back on without the user asking.
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: false },
			frameOrigin,
		);
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive(submission);
		await settled();
		expect(onSend).not.toHaveBeenCalled();
	});

	it("disarms when the frame loads another document", async () => {
		const { frame, receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
		});

		// The app navigated on its own. Whatever the new document sends, it
		// was never told to pick.
		await act(async () => {
			frame().dispatchEvent(new Event("load"));
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		await settled();
		expect(onSend).not.toHaveBeenCalled();
	});

	it("requests the overlay again after the app navigated away from it", async () => {
		const { frame, frameOrigin, receive } = renderPanel(sent(), 0);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const loadedFrame = frame();

		// A navigation without the marker drops the overlay. That is not a
		// failure to load, so the control must not give up.
		await act(async () => {
			loadedFrame.dispatchEvent(new Event("load"));
			await settled();
		});
		expect(annotateButton()).not.toHaveAttribute("aria-disabled", "true");
		expect(annotateButton()).not.toHaveAttribute("aria-busy", "true");

		await userEvent.click(annotateButton());
		expect(frame()).not.toBe(loadedFrame);
		expect(new URL(frame().src).searchParams.get("coder_annotate")).toBe("1");
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		receive({ type: "coder-annotator:ready" });
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
	});

	it("drops malformed submissions", async () => {
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:submit", page: {}, annotations: "nope" },
				origin: frameOrigin,
				source: frameWindow(frame()),
			}),
		);
		await settled();
		expect(onSend).not.toHaveBeenCalled();
	});

	it("ignores messages from other origins or windows", async () => {
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		for (const init of [
			{ origin: "https://evil.example.com", source: frameWindow(frame()) },
			{ origin: frameOrigin, source: window },
		]) {
			window.dispatchEvent(
				new MessageEvent("message", { data: submission, ...init }),
			);
		}
		await settled();
		expect(onSend).not.toHaveBeenCalled();
		// The same submission from the frame itself goes through.
		receive(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
	});

	it("shimmers annotated elements while the agent works on them", async () => {
		const { frame, frameOrigin, receive, onSend, setAgentWorking } =
			renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		receive(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(postMessage).not.toHaveBeenCalledWith(
			expect.objectContaining({ type: "coder-annotator:highlight" }),
			frameOrigin,
		);

		setAgentWorking(true);
		const item = {
			id: "a",
			selector: "#save",
			url: submission.page.url,
		};
		await waitFor(() =>
			expect(postMessage).toHaveBeenCalledWith(
				{ type: "coder-annotator:highlight", items: [item] },
				frameOrigin,
			),
		);

		// A second annotation during the same turn joins the first.
		receive({
			...submission,
			annotations: [{ ...submission.annotations[0], id: "b" }],
		});
		await waitFor(() =>
			expect(postMessage).toHaveBeenCalledWith(
				{
					type: "coder-annotator:highlight",
					items: [item, { ...item, id: "b" }],
				},
				frameOrigin,
			),
		);

		setAgentWorking(false);
		await waitFor(() =>
			expect(postMessage).toHaveBeenLastCalledWith(
				{ type: "coder-annotator:clear-highlights" },
				frameOrigin,
			),
		);
	});

	it("steps aside while the tab is shown in its own window", async () => {
		const open = vi.spyOn(window, "open").mockReturnValue(window);
		const popoutWindow = new BroadcastChannel(tabPopoutChannelName(tab.id));
		const fromChat: TabPopoutMessage[] = [];
		popoutWindow.addEventListener("message", (event) => {
			fromChat.push(event.data as TabPopoutMessage);
		});
		const { onSend } = renderPanel();
		try {
			// The panel asks whether a window is already showing the tab.
			await waitFor(() =>
				expect(fromChat).toContainEqual({ type: "probe" } as TabPopoutMessage),
			);

			await userEvent.click(
				screen.getByRole("button", { name: "Open in a separate window" }),
			);
			expect(open).toHaveBeenCalledWith(
				"/agents/chat-1/tabs/port-3000",
				tabPopoutChannelName(tab.id),
				expect.stringContaining("popup"),
			);

			// The window announces itself; the panel replaces its preview with a
			// placeholder rather than run a second copy of the app.
			popoutWindow.postMessage({
				type: "popout-opened",
			} satisfies TabPopoutMessage);
			await screen.findByRole("button", { name: "Bring back" });
			expect(screen.queryByTitle("Preview :3000")).not.toBeInTheDocument();
			expect(
				screen.queryByRole("button", { name: /annotate elements/i }),
			).not.toBeInTheDocument();
			await waitFor(() =>
				expect(fromChat).toContainEqual({
					type: "chat-state",
					isAgentWorking: false,
				} satisfies TabPopoutMessage),
			);

			// The window sends through this chat's composer.
			popoutWindow.postMessage({
				type: "send",
				id: "send-1",
				message: "# UI annotations",
			} satisfies TabPopoutMessage);
			await waitFor(() =>
				expect(fromChat).toContainEqual({
					type: "send-result",
					id: "send-1",
					result: "sent",
				} satisfies TabPopoutMessage),
			);
			expect(onSend).toHaveBeenCalledWith("# UI annotations");

			// Bring back asks the window to close and restores the preview.
			await userEvent.click(screen.getByRole("button", { name: "Bring back" }));
			await waitFor(() =>
				expect(fromChat).toContainEqual({
					type: "bring-back",
				} satisfies TabPopoutMessage),
			);
			expect(screen.getByTitle("Preview :3000")).toBeInTheDocument();
		} finally {
			popoutWindow.close();
			open.mockRestore();
		}
	});

	it("restores the preview when the window closes on its own", async () => {
		renderPanel();
		const popoutWindow = new BroadcastChannel(tabPopoutChannelName(tab.id));
		try {
			popoutWindow.postMessage({
				type: "popout-opened",
			} satisfies TabPopoutMessage);
			await screen.findByRole("button", { name: "Bring back" });
			popoutWindow.postMessage({
				type: "popout-closed",
			} satisfies TabPopoutMessage);
			await screen.findByTitle("Preview :3000");
		} finally {
			popoutWindow.close();
		}
	});

	it("starts annotating when it is the tab's own window", async () => {
		const onSend = sent();
		renderComponent(
			<ComposerContext value={{ send: onSend }}>
				<PortPreviewPanel
					chatId="chat-1"
					workspace={MockWorkspace}
					agent={MockWorkspaceAgent}
					host="*.apps.example.com"
					tab={tab}
					canAnnotate
					isPopoutWindow
				/>
			</ComposerContext>,
		);
		const frame = screen.getByTitle<HTMLIFrameElement>("Preview :3000");
		expect(new URL(frame.src).searchParams.get("coder_annotate")).toBe("1");
		expect(frame).toHaveAttribute(
			"sandbox",
			expect.stringContaining("allow-scripts"),
		);
		expect(frame.getAttribute("sandbox")).not.toContain("allow-top-navigation");
		expect(annotateButton()).toHaveAttribute("aria-pressed", "true");
		// The app can be opened directly; there is no further window to open.
		expect(screen.getByLabelText("Open port in new tab")).toHaveAttribute(
			"href",
			expect.stringContaining("3000--"),
		);
		expect(
			screen.queryByRole("button", { name: "Open in a separate window" }),
		).not.toBeInTheDocument();

		const postMessage = vi.spyOn(frameWindow(frame), "postMessage");
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:ready" },
				origin: new URL(frame.src).origin,
				source: frameWindow(frame),
			}),
		);
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			new URL(frame.src).origin,
		);
	});

	it("stops requesting the overlay once it failed to load", async () => {
		const { frame, frameOrigin, receive } = renderPanel(sent(), 0);
		await requestOverlay();
		const requestedSrc = frame().src;
		const requestedFrame = frame();
		frame().dispatchEvent(new Event("load"));
		await waitFor(() =>
			expect(annotateButton()).toHaveAttribute("aria-disabled", "true"),
		);
		// Still focusable, so keyboard users get the explanation too.
		expect(annotateButton()).toHaveAccessibleDescription(
			/could not load in this app/,
		);

		await userEvent.click(annotateButton());
		expect(annotateButton()).toHaveFocus();
		await userEvent.keyboard("{Enter} ");
		expect(frame()).toBe(requestedFrame);
		expect(frame().src).toBe(requestedSrc);

		// A late ready no longer delivers the request the control gave up on;
		// it just makes the overlay available to ask again.
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
		});
		expect(postMessage).not.toHaveBeenCalled();
		expect(annotateButton()).not.toHaveAttribute("aria-disabled", "true");
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");

		await userEvent.click(annotateButton());
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
		expect(annotateButton()).toHaveAttribute("aria-pressed", "true");
	});

	it("shows annotate mode on until the dashboard turns it off, whatever the overlay reports", async () => {
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		// The page announces the overlay but never echoes picking. The
		// request stands, so the control must say so.
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "true");
		expect(annotateButton()).toHaveAccessibleName("Stop annotating");

		// And Escape must close it from here without the overlay's help.
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		await userEvent.keyboard("{Escape}");
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: false, hint: true },
			frameOrigin,
		);
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive(submission);
		await settled();
		expect(onSend).not.toHaveBeenCalled();
	});
});
