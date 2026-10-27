import { useEffect } from "react";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { belowMdViewportMediaQuery } from "#/utils/mobile";

/** Keeps portaled mobile menus aligned with the chat composer and keyboard. */
export function useChatDropdownPosition(composerElement: HTMLElement | null) {
	// A custom property change on `<html>` restyles every element in the
	// document, and only the below-md `mobile-full-width-dropdown*` utilities
	// read these variables.
	const isBelowMd = useMediaQuery(belowMdViewportMediaQuery);
	useEffect(() => {
		if (!composerElement || !isBelowMd) return;
		// Fixed Radix menus need layout-viewport coordinates. Clamp to the
		// visual viewport only for positive heights: mobile WebKit can mix
		// coordinate systems while the keyboard settles.
		const viewport = globalThis.visualViewport;
		const root = document.documentElement;
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
		const composerGap = 8;
		const viewportPadding = 16;
		const minimumMenuHeight = 96;
		const update = () => {
			const rect = composerElement.getBoundingClientRect();
			const fixedViewportBottom = fixedProbe.getBoundingClientRect().bottom;
			const visibleViewportTop = viewport?.offsetTop ?? 0;
			const bottom = Math.max(0, fixedViewportBottom - rect.bottom);
			// Keep the dropdown's bottom edge above the software keyboard,
			// which covers the bottom of the layout viewport without moving
			// fixed-positioned elements.
			const keyboardInset = viewport
				? Math.max(
						0,
						fixedViewportBottom - (viewport.offsetTop + viewport.height),
					)
				: 0;
			const aboveComposerBottom = Math.max(
				0,
				fixedViewportBottom - rect.top + composerGap,
				keyboardInset + composerGap,
			);
			const dropdownBottomEdgeTop = fixedViewportBottom - aboveComposerBottom;
			const maxHeightCandidates = [
				dropdownBottomEdgeTop - visibleViewportTop - viewportPadding,
				dropdownBottomEdgeTop - viewportPadding,
			].filter((height) => height > 0);
			const aboveComposerMaxHeight = Math.max(
				minimumMenuHeight,
				maxHeightCandidates.length > 0 ? Math.min(...maxHeightCandidates) : 0,
			);
			root.style.setProperty("--mobile-dropdown-bottom", `${bottom}px`);
			root.style.setProperty("--mobile-dropdown-left", `${rect.left}px`);
			root.style.setProperty("--mobile-dropdown-width", `${rect.width}px`);
			root.style.setProperty(
				"--mobile-dropdown-above-composer-bottom",
				`${aboveComposerBottom}px`,
			);
			root.style.setProperty(
				"--mobile-dropdown-above-composer-max-height",
				`${aboveComposerMaxHeight}px`,
			);
		};
		const animationFrameIDs = new Set<number>();
		const timeoutIDs = new Set<ReturnType<typeof setTimeout>>();
		const cancelScheduledUpdates = () => {
			for (const id of animationFrameIDs) {
				cancelAnimationFrame(id);
			}
			animationFrameIDs.clear();
			for (const id of timeoutIDs) {
				clearTimeout(id);
			}
			timeoutIDs.clear();
		};
		const queueAnimationFrame = (callback: () => void) => {
			const id = requestAnimationFrame(() => {
				animationFrameIDs.delete(id);
				callback();
			});
			animationFrameIDs.add(id);
		};
		const scheduleUpdate = () => {
			cancelScheduledUpdates();
			update();
			// Mobile WebKit can finish keyboard panning after focus and
			// input events. Re-read geometry after the viewport settles so
			// the first slash-menu render is not stuck under the composer.
			queueAnimationFrame(() => {
				update();
				queueAnimationFrame(update);
			});
			for (const delay of [50, 150, 300]) {
				const id = setTimeout(() => {
					timeoutIDs.delete(id);
					update();
				}, delay);
				timeoutIDs.add(id);
			}
		};
		scheduleUpdate();
		const ro = new ResizeObserver(scheduleUpdate);
		ro.observe(composerElement);
		addEventListener("resize", scheduleUpdate);
		addEventListener("scroll", scheduleUpdate, { passive: true });
		addEventListener("focusin", scheduleUpdate);
		addEventListener("focusout", scheduleUpdate);
		composerElement.addEventListener("input", scheduleUpdate);
		composerElement.addEventListener("keyup", scheduleUpdate);
		document.addEventListener("selectionchange", scheduleUpdate);
		viewport?.addEventListener("resize", scheduleUpdate);
		viewport?.addEventListener("scroll", scheduleUpdate);
		viewport?.addEventListener("scrollend", scheduleUpdate);
		return () => {
			ro.disconnect();
			cancelScheduledUpdates();
			removeEventListener("resize", scheduleUpdate);
			removeEventListener("scroll", scheduleUpdate);
			removeEventListener("focusin", scheduleUpdate);
			removeEventListener("focusout", scheduleUpdate);
			composerElement.removeEventListener("input", scheduleUpdate);
			composerElement.removeEventListener("keyup", scheduleUpdate);
			document.removeEventListener("selectionchange", scheduleUpdate);
			viewport?.removeEventListener("resize", scheduleUpdate);
			viewport?.removeEventListener("scroll", scheduleUpdate);
			viewport?.removeEventListener("scrollend", scheduleUpdate);
			fixedProbe.remove();
			root.style.removeProperty("--mobile-dropdown-bottom");
			root.style.removeProperty("--mobile-dropdown-left");
			root.style.removeProperty("--mobile-dropdown-width");
			root.style.removeProperty("--mobile-dropdown-above-composer-bottom");
			root.style.removeProperty("--mobile-dropdown-above-composer-max-height");
		};
	}, [composerElement, isBelowMd]);
}
