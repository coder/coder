import { useLayoutEffect, useState } from "react";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
import { useMediaQuery } from "./useMediaQuery";

/** @internal Computes fixed-position geometry above an anchor and software keyboard. */
export const getMobileMenuPosition = (
	anchor: Pick<DOMRect, "left" | "top" | "width">,
	fixedViewportBottom: number,
	viewport: Pick<VisualViewport, "offsetTop" | "height"> | null,
) => {
	const visibleViewportTop = viewport?.offsetTop ?? 0;
	const keyboardInset = viewport
		? Math.max(0, fixedViewportBottom - (viewport.offsetTop + viewport.height))
		: 0;
	const bottom = Math.max(
		0,
		fixedViewportBottom - anchor.top + 8,
		keyboardInset + 8,
	);
	const bottomEdgeTop = fixedViewportBottom - bottom;
	// WebKit can mix coordinate systems while the keyboard settles.
	// Only positive visual-viewport height candidates can constrain the menu.
	const heightCandidates = [
		bottomEdgeTop - visibleViewportTop - 16,
		bottomEdgeTop - 16,
	].filter((height) => height > 0);
	return {
		left: anchor.left,
		width: anchor.width,
		bottom,
		maxHeight: Math.max(
			96,
			heightCandidates.length > 0 ? Math.min(...heightCandidates) : 0,
		),
	};
};

/** Keeps keyboard-sensitive mobile menus docked to their own anchor while open. */
export const useMobileMenuPosition = (
	anchor: HTMLElement | null | undefined,
	open: boolean,
): HTMLElement | undefined => {
	const isBelowMd = useMediaQuery(belowMdViewportMediaQuery);
	const [container] = useState(() => document.createElement("div"));
	const enabled = Boolean(anchor);
	const active = enabled && isBelowMd && open;

	useLayoutEffect(() => {
		if (!enabled) return;
		document.body.appendChild(container);
		return () => container.remove();
	}, [container, enabled]);

	useLayoutEffect(() => {
		if (!anchor || !active) return;
		document.body.appendChild(container);
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
			if (container.style.getPropertyValue(name) !== next) {
				container.style.setProperty(name, next);
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

		const frames = new Set<number>();
		const timers = new Set<ReturnType<typeof setTimeout>>();
		const cancelUpdates = () => {
			for (const frame of frames) cancelAnimationFrame(frame);
			frames.clear();
			for (const timer of timers) clearTimeout(timer);
			timers.clear();
		};
		const queueFrame = (callback: () => void) => {
			const frame = requestAnimationFrame(() => {
				frames.delete(frame);
				callback();
			});
			frames.add(frame);
		};
		const scheduleUpdate = () => {
			cancelUpdates();
			update();
			// Keyboard panning can finish after focus and viewport events.
			queueFrame(() => {
				update();
				queueFrame(update);
			});
			for (const delay of [50, 150, 300]) {
				const timer = setTimeout(() => {
					timers.delete(timer);
					update();
				}, delay);
				timers.add(timer);
			}
		};
		scheduleUpdate();
		const observer = new ResizeObserver(scheduleUpdate);
		observer.observe(anchor);
		window.addEventListener("resize", scheduleUpdate);
		window.addEventListener("scroll", scheduleUpdate, {
			passive: true,
			capture: true,
		});
		window.addEventListener("focusin", scheduleUpdate);
		window.addEventListener("focusout", scheduleUpdate);
		anchor.addEventListener("input", scheduleUpdate);
		anchor.addEventListener("keyup", scheduleUpdate);
		document.addEventListener("selectionchange", scheduleUpdate);
		viewport?.addEventListener("resize", scheduleUpdate);
		viewport?.addEventListener("scroll", scheduleUpdate);
		viewport?.addEventListener("scrollend", scheduleUpdate);
		return () => {
			observer.disconnect();
			cancelUpdates();
			window.removeEventListener("resize", scheduleUpdate);
			window.removeEventListener("scroll", scheduleUpdate, true);
			window.removeEventListener("focusin", scheduleUpdate);
			window.removeEventListener("focusout", scheduleUpdate);
			anchor.removeEventListener("input", scheduleUpdate);
			anchor.removeEventListener("keyup", scheduleUpdate);
			document.removeEventListener("selectionchange", scheduleUpdate);
			viewport?.removeEventListener("resize", scheduleUpdate);
			viewport?.removeEventListener("scroll", scheduleUpdate);
			viewport?.removeEventListener("scrollend", scheduleUpdate);
			fixedProbe.remove();
		};
	}, [active, anchor, container]);

	return enabled ? container : undefined;
};
