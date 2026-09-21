import { useCallback, useLayoutEffect, useRef, useState } from "react";
import {
	type AnnotationSubmission,
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
 *
 * The returned callbacks are stable so effects can depend on them.
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
	const onSubmitRef = useRef(onSubmit);
	const onRevokeRef = useRef(onRevoke);
	const frameOriginRef = useRef(frameOrigin);
	const readyTimeoutRef = useRef(readyTimeoutMs);
	const enabledRef = useRef(enabled);
	// Layout effects run in the commit, before the browser can deliver the
	// remounted frame's load event, so `frameLoaded` never sees the values
	// from the previous render.
	useLayoutEffect(() => {
		onSubmitRef.current = onSubmit;
		onRevokeRef.current = onRevoke;
		frameOriginRef.current = frameOrigin;
		readyTimeoutRef.current = readyTimeoutMs;
		enabledRef.current = enabled;
	}, [onSubmit, onRevoke, frameOrigin, readyTimeoutMs, enabled]);

	const update = useCallback((patch: Partial<BridgeState>) => {
		stateRef.current = { ...stateRef.current, ...patch };
		setState(stateRef.current);
	}, []);

	const revoke = useCallback(() => {
		if (authorizedRef.current) {
			authorizedRef.current = false;
			onRevokeRef.current?.();
		}
	}, []);

	const post = useCallback(
		(message: HostToAnnotatorMessage) => {
			const frameWindow = frameRef.current?.contentWindow;
			const origin = frameOriginRef.current;
			if (frameWindow && origin) {
				frameWindow.postMessage(message, origin);
			}
		},
		[frameRef],
	);

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
			}, readyTimeoutRef.current);
			return;
		}
		pendingPickingRef.current = false;
		update({
			ready: false,
			unavailable: false,
			picking: false,
			requested: false,
		});
	}, [update, revoke]);

	// Also bound in the commit, so for a remounted frame the old listener is
	// gone and the state reset before its load event can reach `frameLoaded`.
	useLayoutEffect(() => {
		if (!enabled || !frameOrigin) {
			return;
		}
		const frame = frameRef.current;
		const handler = (event: MessageEvent) => {
			const frameWindow = frame?.contentWindow;
			if (
				event.origin !== frameOrigin ||
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
				frameWindow.postMessage(outbound, frameOrigin);
			switch (message.type) {
				case "coder-annotator:ready":
					clearTimeout(readyTimerRef.current);
					update({ ready: true, unavailable: false, loading: false });
					if (pendingPickingRef.current) {
						pendingPickingRef.current = false;
						authorizedRef.current = true;
						send({ type: "coder-annotator:set-picking", picking: true });
					}
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
					if (
						!authorizedRef.current ||
						!fromOrigin(message.page.url, frameOrigin)
					) {
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
			// The frame is being replaced: forget what the old one reported,
			// but keep what the dashboard asked for, since it asked for the
			// replacement and its load is what `frameLoaded` will report next.
			revoke();
			update({ ready: false, unavailable: false, picking: false });
		};
	}, [frameRef, frameKey, frameOrigin, enabled, update, revoke]);

	const setPicking = useCallback(
		(next: boolean) => {
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
				post({ type: "coder-annotator:set-picking", picking: next });
				return;
			}
			pendingPickingRef.current = next;
			update({ requested: next, loading: stateRef.current.loading || next });
		},
		[post, update, revoke],
	);

	return { ...state, setPicking, frameLoaded };
}
