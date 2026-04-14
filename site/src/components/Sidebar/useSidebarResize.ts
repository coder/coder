import { useCallback, useEffect, useRef, useState } from "react";

const DEFAULT_WIDTH = 240;
const COLLAPSED_WIDTH = 56;
const SNAP_THRESHOLD = 100;
const MIN_WIDTH = 56;
const MAX_WIDTH = 480;

function readStoredWidth(key: string): number {
	try {
		const stored = localStorage.getItem(key);
		if (stored !== null) {
			const parsed = Number.parseFloat(stored);
			if (!Number.isNaN(parsed) && parsed >= MIN_WIDTH && parsed <= MAX_WIDTH) {
				return parsed;
			}
		}
	} catch {
		// localStorage may be unavailable in some environments.
	}
	return DEFAULT_WIDTH;
}

function persistWidth(key: string, width: number): void {
	try {
		localStorage.setItem(key, String(width));
	} catch {
		// Silently ignore write failures (e.g. quota exceeded).
	}
}

interface UseSidebarResizeReturn {
	width: number;
	collapsed: boolean;
	onDragStart: (e: React.PointerEvent) => () => void;
}

export function useSidebarResize(
	storageKey = "sidebar-width",
): UseSidebarResizeReturn {
	const [width, setWidth] = useState(() => readStoredWidth(storageKey));
	const rafId = useRef<number | null>(null);

	// Cancel any pending animation frame on unmount to avoid state
	// updates after the component is torn down.
	useEffect(() => {
		return () => {
			if (rafId.current !== null) {
				cancelAnimationFrame(rafId.current);
			}
		};
	}, []);

	const onDragStart = useCallback(
		(e: React.PointerEvent): (() => void) => {
			e.preventDefault();

			// Capture the sidebar's left edge at drag start so the
			// width tracks the absolute cursor position correctly.
			const sidebarRect = (e.currentTarget as HTMLElement)
				.closest("nav")
				?.getBoundingClientRect();
			const startLeft = sidebarRect?.left ?? e.clientX - width;
			const wasCollapsed = width <= COLLAPSED_WIDTH;

			const handlePointerMove = (moveEvent: PointerEvent) => {
				if (rafId.current !== null) {
					cancelAnimationFrame(rafId.current);
				}

				rafId.current = requestAnimationFrame(() => {
					const rawWidth = moveEvent.clientX - startLeft;

					let nextWidth: number;
					if (rawWidth < SNAP_THRESHOLD) {
						// Snap to collapsed when dragged below the threshold.
						nextWidth = COLLAPSED_WIDTH;
					} else if (wasCollapsed && rawWidth >= SNAP_THRESHOLD) {
						// Snap open to the default width when dragging out
						// from the collapsed state past the threshold.
						nextWidth = DEFAULT_WIDTH;
					} else {
						nextWidth = Math.min(Math.max(rawWidth, MIN_WIDTH), MAX_WIDTH);
					}

					setWidth(nextWidth);
					persistWidth(storageKey, nextWidth);
				});
			};

			const cleanup = () => {
				document.removeEventListener("pointermove", handlePointerMove);
				document.removeEventListener("pointerup", handlePointerUp);
				document.body.style.cursor = "";
				document.body.style.userSelect = "";
			};

			const handlePointerUp = () => {
				cleanup();
			};

			// Prevent text selection and show resize cursor globally
			// while dragging so the cursor doesn't flicker.
			document.body.style.cursor = "col-resize";
			document.body.style.userSelect = "none";
			document.addEventListener("pointermove", handlePointerMove);
			document.addEventListener("pointerup", handlePointerUp);

			return cleanup;
		},
		[width, storageKey],
	);

	const collapsed = width <= COLLAPSED_WIDTH;

	return { width, collapsed, onDragStart };
}
