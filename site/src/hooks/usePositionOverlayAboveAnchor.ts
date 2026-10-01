import { useLayoutEffect } from "react";

/** @internal Computes viewport-fixed geometry above an anchor and viewport obstruction. */
export const getOverlayPositionAboveAnchor = (
	anchor: Pick<DOMRect, "left" | "top" | "width">,
	fixedViewportBottom: number,
	viewport: Pick<VisualViewport, "offsetTop" | "height"> | null,
) => {
	const anchorGap = 8;
	const viewportPadding = 16;
	const minimumOverlayHeight = 96;

	const visibleViewportTop = viewport?.offsetTop ?? 0;
	const viewportInset = viewport
		? Math.max(0, fixedViewportBottom - (viewport.offsetTop + viewport.height))
		: 0;

	const bottom = Math.max(
		0,
		fixedViewportBottom - anchor.top + anchorGap,
		viewportInset + anchorGap,
	);
	const bottomEdgeTop = fixedViewportBottom - bottom;

	// WebKit can mix coordinate systems while the keyboard settles.
	// Only positive visual-viewport height candidates can constrain the overlay.
	const heightCandidates = [
		bottomEdgeTop - visibleViewportTop - viewportPadding,
		bottomEdgeTop - viewportPadding,
	].filter((height) => height > 0);

	return {
		left: anchor.left,
		width: anchor.width,
		bottom,
		maxHeight: Math.max(
			minimumOverlayHeight,
			heightCandidates.length > 0 ? Math.min(...heightCandidates) : 0,
		),
	};
};

type UsePositionOverlayAboveAnchorProps = {
	anchorElement: HTMLElement | null | undefined;
	overlayElement: HTMLElement | null | undefined;
	enabled: boolean;
};

/**
 * Owns --anchored-overlay-* while enabled and removes them on cleanup.
 * Callers control positioning CSS; the target must use the viewport as its containing block.
 */
export const usePositionOverlayAboveAnchor = ({
	anchorElement,
	overlayElement,
	enabled,
}: UsePositionOverlayAboveAnchorProps): void => {
	useLayoutEffect(() => {
		if (!anchorElement || !overlayElement || !enabled) {
			return;
		}

		const viewport = window.visualViewport;
		const fixedProbe = document.createElement("div");
		Object.assign(fixedProbe.style, {
			position: "fixed",
			bottom: "0",
			left: "0",
			width: "0",
			height: "0",
			pointerEvents: "none",
			visibility: "hidden",
		});
		document.body.appendChild(fixedProbe);

		const setProperty = (name: string, value: number) => {
			const next = `${value}px`;

			if (overlayElement.style.getPropertyValue(name) !== next) {
				overlayElement.style.setProperty(name, next);
			}
		};

		const update = () => {
			const position = getOverlayPositionAboveAnchor(
				anchorElement.getBoundingClientRect(),
				fixedProbe.getBoundingClientRect().bottom,
				viewport,
			);

			setProperty("--anchored-overlay-left", position.left);
			setProperty("--anchored-overlay-width", position.width);
			setProperty("--anchored-overlay-bottom", position.bottom);
			setProperty("--anchored-overlay-max-height", position.maxHeight);
		};

		update();

		let frame: number | null = null;

		const scheduleUpdate = () => {
			if (frame !== null) {
				return;
			}

			frame = requestAnimationFrame(() => {
				frame = null;
				update();
			});
		};

		const handleScroll = (event: Event) => {
			// Scrolling inside the overlay does not move its anchor.
			if (
				event.target instanceof Node &&
				!event.target.contains(anchorElement)
			) {
				return;
			}

			scheduleUpdate();
		};

		const observer = new ResizeObserver(scheduleUpdate);
		observer.observe(anchorElement);

		window.addEventListener("resize", scheduleUpdate);
		window.addEventListener("scroll", handleScroll, {
			passive: true,
			capture: true,
		});

		viewport?.addEventListener("resize", scheduleUpdate);
		viewport?.addEventListener("scroll", scheduleUpdate);

		return () => {
			observer.disconnect();

			if (frame !== null) {
				cancelAnimationFrame(frame);
			}

			window.removeEventListener("resize", scheduleUpdate);
			window.removeEventListener("scroll", handleScroll, true);

			viewport?.removeEventListener("resize", scheduleUpdate);
			viewport?.removeEventListener("scroll", scheduleUpdate);

			fixedProbe.remove();

			overlayElement.style.removeProperty("--anchored-overlay-left");
			overlayElement.style.removeProperty("--anchored-overlay-width");
			overlayElement.style.removeProperty("--anchored-overlay-bottom");
			overlayElement.style.removeProperty("--anchored-overlay-max-height");
		};
	}, [anchorElement, overlayElement, enabled]);
};
