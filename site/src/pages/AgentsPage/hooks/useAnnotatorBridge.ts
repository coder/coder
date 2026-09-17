import {
	type AnnotationSubmission,
	type HostToAnnotatorMessage,
	parseAnnotatorToHostMessage,
} from "@coder/annotator/protocol";
import {
	type RefObject,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";

interface UseAnnotatorBridgeOptions {
	frameRef: RefObject<HTMLIFrameElement | null>;
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
}

interface AnnotatorBridge {
	ready: boolean;
	// The frame finished loading without the overlay announcing itself.
	unavailable: boolean;
	picking: boolean;
	setPicking: (picking: boolean) => void;
	// Call from the iframe's onLoad. A React prop is attached before the
	// frame can load, unlike a listener added from an effect.
	frameLoaded: () => void;
}

/**
 * Talks to the annotation overlay the app proxy injects into a proxied
 * preview. The overlay is cross-origin and shares its window with the
 * previewed app, so every inbound message is validated and bounded before
 * it reaches the caller, and submissions are only accepted while the
 * dashboard itself has annotate mode switched on.
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
}: UseAnnotatorBridgeOptions): AnnotatorBridge {
	const [ready, setReady] = useState(false);
	const [unavailable, setUnavailable] = useState(false);
	const [picking, setPickingState] = useState(false);
	const readyRef = useRef(false);
	// What the dashboard asked for, as opposed to what the frame reports.
	// The frame can forge state messages but cannot flip this.
	const armedRef = useRef(false);
	// Picking requested before the overlay finished loading; applied once
	// the ready message arrives.
	const pendingPickingRef = useRef<boolean | null>(null);
	const readyTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	const onSubmitRef = useRef(onSubmit);
	const frameOriginRef = useRef(frameOrigin);
	const readyTimeoutRef = useRef(readyTimeoutMs);
	const enabledRef = useRef(enabled);
	useEffect(() => {
		onSubmitRef.current = onSubmit;
		frameOriginRef.current = frameOrigin;
		readyTimeoutRef.current = readyTimeoutMs;
		enabledRef.current = enabled;
	}, [onSubmit, frameOrigin, readyTimeoutMs, enabled]);

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

	const markReady = useCallback((next: boolean) => {
		readyRef.current = next;
		setReady(next);
	}, []);

	// A (re)load starts the clock on the overlay announcing itself. The
	// overlay posts ready after the frame's own load event and postMessage
	// delivery is queued behind it, so this never races a fresh ready.
	const frameLoaded = useCallback(() => {
		if (!enabledRef.current) {
			return;
		}
		markReady(false);
		setPickingState(false);
		clearTimeout(readyTimerRef.current);
		readyTimerRef.current = setTimeout(
			() => setUnavailable(true),
			readyTimeoutRef.current,
		);
	}, [markReady]);

	useEffect(() => {
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
			switch (message.type) {
				case "coder-annotator:ready":
					clearTimeout(readyTimerRef.current);
					markReady(true);
					setUnavailable(false);
					if (pendingPickingRef.current !== null) {
						frameWindow.postMessage(
							{
								type: "coder-annotator:set-picking",
								picking: pendingPickingRef.current,
							} satisfies HostToAnnotatorMessage,
							frameOrigin,
						);
						pendingPickingRef.current = null;
					}
					break;
				case "coder-annotator:state":
					setPickingState(message.picking);
					// The user left annotate mode from inside the preview.
					if (!message.picking) {
						armedRef.current = false;
					}
					break;
				case "coder-annotator:submit": {
					if (!armedRef.current) {
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
			markReady(false);
			setUnavailable(false);
			setPickingState(false);
		};
	}, [frameRef, frameKey, frameOrigin, enabled, markReady]);

	const setPicking = useCallback(
		(next: boolean) => {
			armedRef.current = next;
			if (!readyRef.current) {
				pendingPickingRef.current = next;
				return;
			}
			post({ type: "coder-annotator:set-picking", picking: next });
		},
		[post],
	);

	return { ready, unavailable, picking, setPicking, frameLoaded };
}
