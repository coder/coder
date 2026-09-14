import { type RefObject, useEffect, useRef, useState } from "react";
import {
	type AnnotationSubmission,
	type HostToAnnotatorMessage,
	parseAnnotatorToHostMessage,
} from "#/annotator/protocol";

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
	onSubmit: (submission: AnnotationSubmission) => void;
}

interface AnnotatorBridge {
	ready: boolean;
	picking: boolean;
	count: number;
	setPicking: (picking: boolean) => void;
	clear: () => void;
}

/**
 * Talks to the annotation overlay the app proxy injects into a proxied
 * preview. The overlay is cross-origin and shares its window with the
 * previewed app, so every inbound message is validated and bounded before
 * it reaches the caller.
 */
export function useAnnotatorBridge({
	frameRef,
	frameKey,
	frameOrigin,
	enabled,
	onSubmit,
}: UseAnnotatorBridgeOptions): AnnotatorBridge {
	const [ready, setReady] = useState(false);
	const [picking, setPickingState] = useState(false);
	const [count, setCount] = useState(0);
	// Picking requested before the overlay finished loading; applied once
	// the ready message arrives.
	const pendingPickingRef = useRef<boolean | null>(null);
	const onSubmitRef = useRef(onSubmit);
	useEffect(() => {
		onSubmitRef.current = onSubmit;
	}, [onSubmit]);

	const post = (message: HostToAnnotatorMessage) => {
		const frameWindow = frameRef.current?.contentWindow;
		if (frameWindow && frameOrigin) {
			frameWindow.postMessage(message, frameOrigin);
		}
	};

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
					setReady(true);
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
					setCount(message.count);
					break;
				case "coder-annotator:submit": {
					const { type: _type, ...submission } = message;
					onSubmitRef.current(submission);
					break;
				}
			}
		};
		// The overlay announces itself after the frame's own load event, and
		// postMessage delivery is queued behind it, so resetting here never
		// races a fresh ready message.
		const onFrameLoad = () => {
			setReady(false);
			setPickingState(false);
			setCount(0);
		};
		window.addEventListener("message", handler);
		frame?.addEventListener("load", onFrameLoad);
		return () => {
			window.removeEventListener("message", handler);
			frame?.removeEventListener("load", onFrameLoad);
		};
	}, [frameRef, frameKey, frameOrigin, enabled]);

	return {
		ready,
		picking,
		count,
		setPicking: (next) => {
			if (!ready) {
				pendingPickingRef.current = next;
				return;
			}
			post({ type: "coder-annotator:set-picking", picking: next });
		},
		clear: () => post({ type: "coder-annotator:clear" }),
	};
}
