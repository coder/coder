import { useLayoutEffect, useState } from "react";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
import { useMediaQuery } from "./useMediaQuery";

/** @internal Computes fixed-position geometry above an anchor and software keyboard. */
export const getMobileMenuPosition = (
	anchor: Pick<DOMRect, "left" | "top" | "width">,
	fixedViewportBottom: number,
	viewport: Pick<VisualViewport, "offsetTop" | "height"> | null,
) => {
	const composerGap = 8;
	const viewportPadding = 16;
	const minimumMenuHeight = 96;

	const visibleViewportTop = viewport?.offsetTop ?? 0;
	const keyboardInset = viewport
		? Math.max(0, fixedViewportBottom - (viewport.offsetTop + viewport.height))
		: 0;

	const bottom = Math.max(
		0,
		fixedViewportBottom - anchor.top + composerGap,
		keyboardInset + composerGap,
	);
	const bottomEdgeTop = fixedViewportBottom - bottom;

	// WebKit can mix coordinate systems while the keyboard settles.
	// Only positive visual-viewport height candidates can constrain the menu.
	const heightCandidates = [
		bottomEdgeTop - visibleViewportTop - viewportPadding,
		bottomEdgeTop - viewportPadding,
	].filter((height) => height > 0);

	return {
		left: anchor.left,
		width: anchor.width,
		bottom,
		maxHeight: Math.max(
			minimumMenuHeight,
			heightCandidates.length > 0 ? Math.min(...heightCandidates) : 0,
		),
	};
};

/** Returns a content ref that docks keyboard-sensitive mobile menus while open. */
export const useMobileMenuPosition = (
	anchor: HTMLElement | null | undefined,
	open: boolean,
): React.RefCallback<HTMLDivElement> => {
	const isBelowMd = useMediaQuery(belowMdViewportMediaQuery);
	const [content, setContent] = useState<HTMLDivElement | null>(null);
	const active = isBelowMd && open;

	useLayoutEffect(() => {
		if (!anchor || !content || !active) {
			return;
		}

		// The positioning CSS targets this same Radix wrapper.
		const wrapper = content.closest<HTMLElement>(
			"[data-radix-popper-content-wrapper]",
		);

		if (!wrapper) {
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

			if (wrapper.style.getPropertyValue(name) !== next) {
				wrapper.style.setProperty(name, next);
			}
		};

		const update = () => {
			const position = getMobileMenuPosition(
				anchor.getBoundingClientRect(),
				fixedProbe.getBoundingClientRect().bottom,
				viewport,
			);

			setProperty("--mobile-menu-left", position.left);
			setProperty("--mobile-menu-width", position.width);
			setProperty("--mobile-menu-bottom", position.bottom);
			setProperty("--mobile-menu-max-height", position.maxHeight);
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
			// Scrolling inside a menu or editor does not move the composer.
			if (event.target instanceof Node && !event.target.contains(anchor)) {
				return;
			}

			scheduleUpdate();
		};

		const observer = new ResizeObserver(scheduleUpdate);
		observer.observe(anchor);

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
		};
	}, [active, anchor, content]);

	return setContent;
};
