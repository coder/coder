import {
	type AnnotationSubmission,
	type HighlightItem,
	type HostToAnnotatorMessage,
	parseAnnotatorToHostMessage,
} from "@coder/annotator/protocol";
import { useCallback, useEffect, useRef, useState } from "react";

interface UseAnnotatorBridgeOptions {
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
	highlight: (items: HighlightItem[]) => void;
	clearHighlights: () => void;
}

/**
 * Talks to the annotation overlay the app proxy injects into a proxied
 * preview, whether it lives in the right panel's iframe or in a popout
 * window. The overlay is cross-origin and shares its window with the
 * previewed app, so every inbound message is validated and bounded before
 * it reaches the caller, and submissions are only accepted while the
 * dashboard itself has annotate mode switched on.
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
	const getTargetRef = useRef(getTargetWindow);
	const targetOriginRef = useRef(targetOrigin);
	const readyTimeoutRef = useRef(readyTimeoutMs);
	const enabledRef = useRef(enabled);
	useEffect(() => {
		onSubmitRef.current = onSubmit;
		getTargetRef.current = getTargetWindow;
		targetOriginRef.current = targetOrigin;
		readyTimeoutRef.current = readyTimeoutMs;
		enabledRef.current = enabled;
	}, [onSubmit, getTargetWindow, targetOrigin, readyTimeoutMs, enabled]);

	const post = useCallback((message: HostToAnnotatorMessage) => {
		const target = getTargetRef.current();
		const origin = targetOriginRef.current;
		if (target && origin) {
			target.postMessage(message, origin);
		}
	}, []);

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
		if (!enabled || !targetOrigin) {
			return;
		}
		// A new target inherits the dashboard's intent: if annotate mode was
		// on, the new overlay is asked to pick as soon as it is ready.
		if (armedRef.current) {
			pendingPickingRef.current = true;
		}
		if (!targetIsFrame) {
			frameLoaded();
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
			switch (message.type) {
				case "coder-annotator:ready":
					clearTimeout(readyTimerRef.current);
					markReady(true);
					setUnavailable(false);
					if (pendingPickingRef.current !== null) {
						target.postMessage(
							{
								type: "coder-annotator:set-picking",
								picking: pendingPickingRef.current,
							} satisfies HostToAnnotatorMessage,
							targetOrigin,
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
	}, [targetKey, targetOrigin, targetIsFrame, enabled, markReady, frameLoaded]);

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

	const highlight = useCallback(
		(items: HighlightItem[]) =>
			post({ type: "coder-annotator:highlight", items }),
		[post],
	);
	const clearHighlights = useCallback(
		() => post({ type: "coder-annotator:clear-highlights" }),
		[post],
	);

	return {
		ready,
		unavailable,
		picking,
		setPicking,
		frameLoaded,
		highlight,
		clearHighlights,
	};
}
