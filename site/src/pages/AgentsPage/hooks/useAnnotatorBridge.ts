import { useCallback, useLayoutEffect, useRef, useState } from "react";
import {
	type AnnotationSubmission,
	type HighlightItem,
	type HostToAnnotatorMessage,
	parseAnnotatorToHostMessage,
} from "#/annotator/protocol";

type UseAnnotatorBridgeOptions = {
	// Resolves the window hosting the overlay: the preview iframe's content
	// window, or a popout the dashboard opened. Read through a ref, so a new
	// function identity per render does not rebind anything.
	getTargetWindow: () => Window | null | undefined;
	// Changes whenever the target is replaced (iframe remount, popout opened
	// or closed) so listeners rebind and state resets.
	targetKey: number;
	// Origin the target is expected to load from. Messages from any other
	// origin or window are ignored.
	targetOrigin: string | undefined;
	// Whether the target is an iframe, whose load event (`frameLoaded`)
	// starts the unavailable timeout. A popout has no observable load, so
	// its timeout starts when it becomes the target.
	targetIsFrame: boolean;
	// Nothing is listened to until the user has asked for the overlay, so a
	// preview that was never annotated cannot talk to the dashboard.
	enabled: boolean;
	// How long to wait for the overlay before declaring it unavailable
	// (blocked by CSP, non-HTML page, and so on).
	readyTimeoutMs?: number;
	onSubmit: (submission: AnnotationSubmission) => void;
};

// Remembered across chats and sessions; the hint only needs to land once.
const hintDismissedKey = "coder.annotator.hint-dismissed";

function hintDismissed(): boolean {
	try {
		return window.localStorage.getItem(hintDismissedKey) === "1";
	} catch {
		return true;
	}
}

function rememberHintDismissed() {
	try {
		window.localStorage.setItem(hintDismissedKey, "1");
	} catch {
		// Storage unavailable; the hint simply shows again next time.
	}
}

type BridgeState = {
	// The overlay in the target's current document announced itself.
	ready: boolean;
	// A requested overlay never announced itself within the timeout.
	unavailable: boolean;
	// The dashboard asked to pick while the overlay was not ready and is
	// waiting for it to load. Cleared by ready or the timeout, not by the
	// user changing their mind, so a load in flight is never repeated.
	loading: boolean;
	// Picking as the overlay reports it.
	picking: boolean;
	// Picking as the dashboard wants it: on from `setPicking(true)` until
	// the dashboard or the user in the preview turns it off, or the frame
	// loads a document the dashboard did not ask for.
	requested: boolean;
};

const idleState: BridgeState = {
	ready: false,
	unavailable: false,
	loading: false,
	picking: false,
	requested: false,
};

type AnnotatorBridge = BridgeState & {
	setPicking: (picking: boolean) => void;
	// Call from the iframe's onLoad. A React prop is attached before the
	// frame can load, unlike a listener added from an effect.
	frameLoaded: () => void;
	highlight: (items: HighlightItem[]) => void;
	clearHighlights: () => void;
};

/**
 * Talks to the annotation overlay the app proxy injects into a proxied
 * preview, whether it lives in the right panel's iframe or in a popout
 * window. The overlay is cross-origin and shares its window with the
 * previewed app, so every inbound message is validated and bounded before
 * it reaches the caller, and submissions are only accepted while the
 * dashboard itself has switched annotate mode on for the document
 * currently in the target. The target can forge any message, so nothing
 * it sends ever grants that authorization; the most it can do is give it
 * up.
 *
 * Callers own the frame: when `setPicking(true)` is called while the
 * overlay is neither ready nor loading, they must reload the frame with
 * the marker parameter so the proxy injects the overlay. A popout is
 * opened to annotate in, so it is asked to pick as soon as it is ready;
 * when it hands back to the frame the session ends, since the frame's
 * overlay, if it is still there, will not announce itself again.
 *
 * The returned callbacks are stable so effects can depend on them.
 */
export function useAnnotatorBridge({
	getTargetWindow,
	targetKey,
	targetOrigin,
	targetIsFrame,
	enabled,
	readyTimeoutMs = 5000,
	onSubmit,
}: UseAnnotatorBridgeOptions): AnnotatorBridge {
	const [state, setState] = useState(idleState);
	// Mirror for the callbacks, which run outside render.
	const stateRef = useRef(idleState);
	// Whether submissions from the current document are accepted. Granted
	// only when the dashboard tells that document to pick; revoked when the
	// dashboard or the user stops picking, and whenever the frame loads.
	const authorizedRef = useRef(false);
	// Picking requested before the overlay was ready; delivered, and the
	// authorization granted, once its ready message arrives.
	const pendingPickingRef = useRef(false);
	const previousTargetIsFrameRef = useRef(true);
	const readyTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	const onSubmitRef = useRef(onSubmit);
	const getTargetRef = useRef(getTargetWindow);
	const targetOriginRef = useRef(targetOrigin);
	const readyTimeoutRef = useRef(readyTimeoutMs);
	const enabledRef = useRef(enabled);
	// Layout effects run in the commit, before the browser can deliver the
	// remounted frame's load event, so `frameLoaded` never sees the values
	// from the previous render.
	useLayoutEffect(() => {
		onSubmitRef.current = onSubmit;
		getTargetRef.current = getTargetWindow;
		targetOriginRef.current = targetOrigin;
		readyTimeoutRef.current = readyTimeoutMs;
		enabledRef.current = enabled;
	}, [onSubmit, getTargetWindow, targetOrigin, readyTimeoutMs, enabled]);

	const update = useCallback((patch: Partial<BridgeState>) => {
		stateRef.current = { ...stateRef.current, ...patch };
		setState(stateRef.current);
	}, []);

	const post = useCallback((message: HostToAnnotatorMessage) => {
		const target = getTargetRef.current();
		const origin = targetOriginRef.current;
		if (target && origin) {
			target.postMessage(message, origin);
		}
	}, []);

	// Every load is a new document, which nothing has authorized yet. A
	// load the dashboard asked for starts the clock on the overlay
	// announcing itself; the overlay posts ready after the frame's own load
	// event and postMessage delivery is queued behind it, so this never
	// races a fresh ready. Any other load is the app navigating on its own:
	// the dashboard's intent does not carry over, and the overlay, if the
	// new document has one, will announce itself without a timeout.
	const frameLoaded = useCallback(() => {
		if (!enabledRef.current) {
			return;
		}
		authorizedRef.current = false;
		clearTimeout(readyTimerRef.current);
		if (stateRef.current.loading) {
			update({ ready: false, picking: false });
			readyTimerRef.current = setTimeout(
				() => update({ loading: false, unavailable: true }),
				readyTimeoutRef.current,
			);
			return;
		}
		pendingPickingRef.current = false;
		update({
			ready: false,
			unavailable: false,
			picking: false,
			requested: false,
		});
	}, [update]);

	// Also bound in the commit, so for a remounted frame the old listener is
	// gone and the state reset before its load event can reach `frameLoaded`.
	useLayoutEffect(() => {
		if (!enabled || !targetOrigin) {
			return;
		}
		const handback = !previousTargetIsFrameRef.current && targetIsFrame;
		previousTargetIsFrameRef.current = targetIsFrame;
		if (!targetIsFrame) {
			// A popout is opened to annotate in: ask it to pick once ready.
			pendingPickingRef.current = true;
			update({ requested: true, loading: true });
			frameLoaded();
		} else if (handback) {
			// The frame was not reloaded, so nothing is loading and its overlay
			// will not announce itself again. The next request reloads it.
			pendingPickingRef.current = false;
			update({ requested: false, loading: false });
		}
		const handler = (event: MessageEvent) => {
			const target = getTargetRef.current();
			if (event.origin !== targetOrigin || !target || event.source !== target) {
				return;
			}
			const message = parseAnnotatorToHostMessage(event.data);
			if (!message) {
				return;
			}
			const send = (outbound: HostToAnnotatorMessage) =>
				target.postMessage(outbound, targetOrigin);
			switch (message.type) {
				case "coder-annotator:ready":
					clearTimeout(readyTimerRef.current);
					update({ ready: true, unavailable: false, loading: false });
					if (pendingPickingRef.current) {
						pendingPickingRef.current = false;
						authorizedRef.current = true;
						send({
							type: "coder-annotator:set-picking",
							picking: true,
							hint: !hintDismissed(),
						});
					}
					break;
				case "coder-annotator:hint-dismissed":
					rememberHintDismissed();
					break;
				case "coder-annotator:state":
					if (message.picking) {
						// Picking only starts from the dashboard. Anything else
						// claiming to pick is told to stop and not mirrored.
						if (!authorizedRef.current) {
							send({ type: "coder-annotator:set-picking", picking: false });
							return;
						}
						update({ picking: true });
						return;
					}
					// A stop after picking was confirmed is the user leaving
					// annotate mode from inside the preview. Before that it is
					// the overlay reporting its initial state, which must not
					// undo a request the dashboard has just delivered.
					if (stateRef.current.picking) {
						authorizedRef.current = false;
						update({ picking: false, requested: false });
					}
					break;
				case "coder-annotator:submit": {
					if (!authorizedRef.current) {
						return;
					}
					const { type: _type, ...submission } = message;
					onSubmitRef.current(submission);
					break;
				}
			}
		};
		window.addEventListener("message", handler);
		return () => {
			clearTimeout(readyTimerRef.current);
			window.removeEventListener("message", handler);
			// The target is being replaced: forget what the old one reported,
			// but keep what the dashboard asked for, since it asked for the
			// replacement and its load is what `frameLoaded` will report next.
			authorizedRef.current = false;
			update({ ready: false, unavailable: false, picking: false });
		};
	}, [targetKey, targetOrigin, targetIsFrame, enabled, update, frameLoaded]);

	const setPicking = useCallback(
		(next: boolean) => {
			if (stateRef.current.ready) {
				authorizedRef.current = next;
				update({ requested: next });
				post({
					type: "coder-annotator:set-picking",
					picking: next,
					hint: !hintDismissed(),
				});
				return;
			}
			pendingPickingRef.current = next;
			update({ requested: next, loading: stateRef.current.loading || next });
		},
		[post, update],
	);

	const highlight = useCallback(
		(items: HighlightItem[]) =>
			post({ type: "coder-annotator:highlight", items }),
		[post],
	);
	const clearHighlights = useCallback(
		() => post({ type: "coder-annotator:clear-highlights" }),
		[post],
	);

	return { ...state, setPicking, frameLoaded, highlight, clearHighlights };
}
