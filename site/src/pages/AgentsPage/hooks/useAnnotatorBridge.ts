import { useEffectEvent, useLayoutEffect, useRef, useState } from "react";
import {
	type AnnotationSubmission,
	type HighlightItem,
	type HostToAnnotatorMessage,
	parseAnnotatorToHostMessage,
} from "#/annotator/protocol";

type UseAnnotatorBridgeOptions = {
	frameRef: React.RefObject<HTMLIFrameElement | null>;
	// Changes whenever the iframe element is remounted so listeners rebind.
	frameKey: number;
	// Origin the iframe is expected to load from. Messages from any other
	// origin or window are ignored.
	frameOrigin: string | undefined;
	// Nothing is listened to until the user has asked for the overlay, so a
	// preview that was never annotated cannot talk to the dashboard.
	enabled: boolean;
	// How long after the frame loads to wait for the overlay before
	// declaring it unavailable (blocked by CSP, non-HTML page, and so on).
	readyTimeoutMs?: number;
	onSubmit: (submission: AnnotationSubmission) => void;
	// Called when the dashboard withdraws the page's authorization: annotate
	// mode turned off from either side, or the frame loaded a document. Lets
	// callers drop work queued on the page's behalf.
	onRevoke?: () => void;
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
	// The overlay in the frame's current document announced itself.
	ready: boolean;
	// A requested overlay never announced itself within the timeout.
	unavailable: boolean;
	// The dashboard asked to pick while the overlay was not ready and is
	// waiting for it to load. Cleared by ready or the timeout, not by the
	// user changing their mind, so a load in flight is never repeated.
	loading: boolean;
	// Picking as the overlay reports it, cleared early when the dashboard
	// asks it to stop.
	picking: boolean;
	// Picking as the dashboard wants it: on from `setPicking(true)` until
	// the dashboard or the user in the preview turns it off, the frame
	// loads a document the dashboard did not ask for, or the overlay fails
	// to load. Submissions are only ever accepted while this is on, so the
	// control shows it, rather than anything the page reports.
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
	// Ends the working state: the listed annotations are acknowledged as
	// changed, the rest are cleared.
	resolveHighlights: (ids: string[]) => void;
};

// Only a document at the frame's origin can host the overlay, so a
// submission claiming a page elsewhere is not one the user made there.
function fromOrigin(pageUrl: string, origin: string): boolean {
	try {
		return new URL(pageUrl).origin === origin;
	} catch {
		return false;
	}
}

/**
 * Talks to the annotation overlay the app proxy injects into a proxied
 * preview. The overlay is cross-origin and shares its window with the
 * previewed app, so every inbound message is validated and bounded before
 * it reaches the caller, and submissions are only accepted while the
 * dashboard itself has switched annotate mode on for the document
 * currently in the frame. The frame can forge any message, so nothing it
 * sends ever grants that authorization; the most it can do is give it up.
 *
 * Callers own the frame: when `setPicking(true)` is called while the
 * overlay is neither ready nor loading, they must reload the frame with
 * the marker parameter so the proxy injects the overlay.
 */
export function useAnnotatorBridge({
	frameRef,
	frameKey,
	frameOrigin,
	enabled,
	readyTimeoutMs = 5000,
	onSubmit,
	onRevoke,
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
	const readyTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	const update = (patch: Partial<BridgeState>) => {
		stateRef.current = { ...stateRef.current, ...patch };
		setState(stateRef.current);
	};

	const revoke = () => {
		if (authorizedRef.current) {
			authorizedRef.current = false;
			onRevoke?.();
		}
	};

	const post = (message: HostToAnnotatorMessage) => {
		const frameWindow = frameRef.current?.contentWindow;
		if (frameWindow && frameOrigin) {
			frameWindow.postMessage(message, frameOrigin);
		}
	};

	// Every load is a new document, which nothing has authorized yet. A
	// load the dashboard asked for starts the clock on the overlay
	// announcing itself; the overlay posts ready after the frame's own load
	// event and postMessage delivery is queued behind it, so this never
	// races a fresh ready. Any other load is the app navigating on its own:
	// the dashboard's intent does not carry over, and the overlay, if the
	// new document has one, will announce itself without a timeout.
	const frameLoaded = () => {
		if (!enabled) {
			return;
		}
		revoke();
		clearTimeout(readyTimerRef.current);
		if (stateRef.current.loading) {
			update({ ready: false, picking: false });
			// Giving up withdraws the request too: the control is about to say
			// the overlay is unavailable, so a ready arriving after that must
			// not quietly deliver it. The user asks again instead.
			readyTimerRef.current = setTimeout(() => {
				pendingPickingRef.current = false;
				update({ loading: false, unavailable: true, requested: false });
			}, readyTimeoutMs);
			return;
		}
		pendingPickingRef.current = false;
		update({
			ready: false,
			unavailable: false,
			picking: false,
			requested: false,
		});
	};

	const receiveMessage = useEffectEvent(
		(event: MessageEvent, frame: HTMLIFrameElement | null, origin: string) => {
			const frameWindow = frame?.contentWindow;
			if (
				event.origin !== origin ||
				!frameWindow ||
				event.source !== frameWindow
			) {
				return;
			}
			const message = parseAnnotatorToHostMessage(event.data);
			if (!message) {
				return;
			}
			const send = (outbound: HostToAnnotatorMessage) =>
				frameWindow.postMessage(outbound, origin);
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
						revoke();
						update({ picking: false, requested: false });
					}
					break;
				case "coder-annotator:submit": {
					if (!authorizedRef.current || !fromOrigin(message.page.url, origin)) {
						return;
					}
					const { type: _type, ...submission } = message;
					onSubmit(submission);
					break;
				}
			}
		},
	);

	const resetFrame = useEffectEvent(() => {
		revoke();
		update({ ready: false, unavailable: false, picking: false });
	});

	// Bind before the remounted frame can load. Callback changes must not
	// end an annotation session, but incoming messages need current handlers.
	useLayoutEffect(() => {
		if (!enabled || !frameOrigin) {
			return;
		}
		const frame = frameRef.current;
		const handler = (event: MessageEvent) =>
			receiveMessage(event, frame, frameOrigin);
		window.addEventListener("message", handler);
		return () => {
			clearTimeout(readyTimerRef.current);
			window.removeEventListener("message", handler);
			// The frame is being replaced: forget what the old one reported,
			// but keep what the dashboard asked for, since it asked for the
			// replacement and its load is what `frameLoaded` will report next.
			resetFrame();
		};
	}, [frameRef, frameKey, frameOrigin, enabled]);

	const setPicking = (next: boolean) => {
		if (stateRef.current.ready) {
			if (next) {
				authorizedRef.current = true;
				update({ requested: true });
			} else {
				revoke();
				// A stop takes effect now rather than when the overlay echoes
				// it, so an echo arriving after the user has asked again is not
				// mistaken for them leaving from inside the preview.
				update({ requested: false, picking: false });
			}
			post({
				type: "coder-annotator:set-picking",
				picking: next,
				hint: !hintDismissed(),
			});
			return;
		}
		pendingPickingRef.current = next;
		update({ requested: next, loading: stateRef.current.loading || next });
	};

	const highlight = (items: HighlightItem[]) =>
		post({ type: "coder-annotator:highlight", items });
	const clearHighlights = () =>
		post({ type: "coder-annotator:clear-highlights" });
	const resolveHighlights = (ids: string[]) =>
		post({ type: "coder-annotator:resolved", ids });

	return {
		...state,
		setPicking,
		frameLoaded,
		highlight,
		clearHighlights,
		resolveHighlights,
	};
}
