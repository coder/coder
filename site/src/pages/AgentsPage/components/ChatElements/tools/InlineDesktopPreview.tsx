import { PanelRightOpenIcon } from "lucide-react";
import type React from "react";
import { useEffect, useRef, useState } from "react";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	type UseDesktopConnectionResult,
	useDesktopConnection,
} from "#/pages/AgentsPage/hooks/useDesktopConnection";
import { DEFAULT_ASPECT, PREVIEW_HEIGHT } from "./previewConstants";

/**
 * Non-interactive inline VNC desktop preview. The noVNC canvas is
 * blocked from receiving pointer/keyboard events so it acts as a
 * read-only thumbnail. An invisible overlay captures clicks and
 * forwards them to `onClick` (e.g. opens the sidebar Desktop tab).
 *
 * The container's aspect-ratio is derived from the remote desktop's
 * framebuffer dimensions so there is no dead space around the
 * preview.
 */
export const InlineDesktopPreview: React.FC<{
	chatId: string;
	onClick?: () => void;
	/** Optional override for the desktop connection hook result.
	 * When provided, the real hook is skipped entirely. Used by
	 * Storybook stories to inject mock connection states without
	 * relying on module-level spies. */
	connectionOverride?: UseDesktopConnectionResult;
}> = ({ chatId, onClick, connectionOverride }) => {
	// Pass undefined chatId when the override is provided so the
	// real hook skips its WebSocket connection logic entirely.
	const realConnection = useDesktopConnection({
		chatId: connectionOverride ? undefined : chatId,
		activated: true,
	});
	const { status, attach } = connectionOverride ?? realConnection;
	const [aspectRatio, setAspectRatio] = useState(DEFAULT_ASPECT);
	const containerRef = useRef<HTMLElement | null>(null);

	// Derive the aspect ratio from the noVNC canvas once connected.
	// noVNC renders into a <canvas> whose intrinsic width/height
	// attributes match the remote framebuffer dimensions (when
	// clipViewport is disabled, which is the case here since
	// scaleViewport is enabled). Querying the canvas from the DOM
	// avoids accessing noVNC's private _fbWidth/_fbHeight fields.
	useEffect(() => {
		if (status !== "connected" || !containerRef.current) {
			return;
		}

		let timeoutId: ReturnType<typeof setTimeout> | null = null;

		const readDimensions = () => {
			const canvas = containerRef.current?.querySelector("canvas");
			if (canvas && canvas.width > 0 && canvas.height > 0) {
				setAspectRatio(`${canvas.width} / ${canvas.height}`);
				return true;
			}
			return false;
		};

		if (!readDimensions()) {
			// The canvas dimensions may not be set immediately after
			// the status transitions to "connected". Retry once after
			// a short delay as a fallback.
			timeoutId = setTimeout(readDimensions, 500);
		}

		return () => {
			if (timeoutId !== null) {
				clearTimeout(timeoutId);
			}
		};
	}, [status]);

	const wrapWithOverlay = (children: React.ReactNode) => (
		<div className="group/preview relative">
			{children}
			{/* Transparent overlay — dims the preview on hover and shows
			    a "View desktop" label so it's clear clicking opens the
			    sidebar desktop tab. */}
			{onClick && (
				<button
					type="button"
					onClick={onClick}
					aria-label="Open desktop tab"
					className="absolute inset-0 z-10 flex cursor-pointer items-center justify-center gap-1.5 border-0 bg-black/0 p-0 transition-colors group-hover/preview:bg-black/50"
				>
					<span className="text-[13px] font-medium text-white opacity-0 drop-shadow-md transition-opacity group-hover/preview:opacity-100">
						View desktop
					</span>
					<PanelRightOpenIcon className="h-4 w-4 text-white opacity-0 drop-shadow-md transition-opacity group-hover/preview:opacity-100" />
				</button>
			)}
		</div>
	);

	if (status === "idle" || status === "connecting") {
		return wrapWithOverlay(
			<div
				className="flex items-center justify-center text-content-secondary"
				style={{ aspectRatio: DEFAULT_ASPECT, height: PREVIEW_HEIGHT }}
			>
				<Spinner loading className="h-5 w-5" />
			</div>,
		);
	}

	if (status === "disconnected") {
		return wrapWithOverlay(
			<div
				className="flex items-center justify-center text-xs text-content-secondary"
				style={{ aspectRatio, height: PREVIEW_HEIGHT }}
			>
				Desktop disconnected. Reconnecting…
			</div>,
		);
	}

	if (status === "error") {
		return wrapWithOverlay(
			<div
				className="flex items-center justify-center text-xs text-content-secondary"
				style={{ aspectRatio: DEFAULT_ASPECT, height: PREVIEW_HEIGHT }}
			>
				Could not connect to desktop.
			</div>,
		);
	}

	// status === "connected" — pointer-events-none on the VNC
	// container prevents noVNC from capturing any input.
	return wrapWithOverlay(
		<div
			ref={(el) => {
				containerRef.current = el;
				if (el) attach(el);
			}}
			className="pointer-events-none"
			style={{ aspectRatio, height: PREVIEW_HEIGHT }}
		/>,
	);
};
