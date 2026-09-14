import { type RefObject, useEffect, useRef, useState } from "react";
import {
	type AnnotationSubmission,
	type HostToAnnotatorMessage,
	isAnnotatorToHostMessage,
} from "#/annotator/protocol";

interface UseAnnotatorBridgeOptions {
	frameRef: RefObject<HTMLIFrameElement | null>;
	// Origin the iframe is expected to load from. Messages from any other
	// origin or window are ignored.
	frameOrigin: string | undefined;
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
 * preview. The overlay is cross-origin, so all state flows over
 * postMessage and is mirrored here for the toolbar.
 */
export function useAnnotatorBridge({
	frameRef,
	frameOrigin,
	onSubmit,
}: UseAnnotatorBridgeOptions): AnnotatorBridge {
	const [ready, setReady] = useState(false);
	const [picking, setPickingState] = useState(false);
	const [count, setCount] = useState(0);
	// Picking requested before the overlay finished loading; applied once
	// the ready message arrives.
	const pendingPickingRef = useRef<boolean | null>(null);

	const post = (message: HostToAnnotatorMessage) => {
		const frameWindow = frameRef.current?.contentWindow;
		if (frameWindow && frameOrigin) {
			frameWindow.postMessage(message, frameOrigin);
		}
	};

	useEffect(() => {
		if (!frameOrigin) {
			return;
		}
		const handler = (event: MessageEvent) => {
			const frameWindow = frameRef.current?.contentWindow;
			if (
				event.origin !== frameOrigin ||
				!frameWindow ||
				event.source !== frameWindow ||
				!isAnnotatorToHostMessage(event.data)
			) {
				return;
			}
			switch (event.data.type) {
				case "coder-annotator:ready":
					setReady(true);
					if (pendingPickingRef.current !== null) {
						post({
							type: "coder-annotator:set-picking",
							picking: pendingPickingRef.current,
						});
						pendingPickingRef.current = null;
					}
					break;
				case "coder-annotator:state":
					setPickingState(event.data.picking);
					setCount(event.data.count);
					break;
				case "coder-annotator:submit": {
					const { type: _type, ...submission } = event.data;
					onSubmit(submission);
					break;
				}
			}
		};
		window.addEventListener("message", handler);
		// The overlay announces itself after the frame's own load event, and
		// postMessage delivery is queued behind it, so resetting here never
		// races a fresh ready message.
		const frame = frameRef.current;
		const onFrameLoad = () => {
			setReady(false);
			setPickingState(false);
			setCount(0);
		};
		frame?.addEventListener("load", onFrameLoad);
		return () => {
			window.removeEventListener("message", handler);
			frame?.removeEventListener("load", onFrameLoad);
		};
	}, [frameRef, frameOrigin, onSubmit, post]);

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
