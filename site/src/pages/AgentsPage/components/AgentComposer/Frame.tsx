import { cn } from "cn";
import type React from "react";
import { useEffect, useState } from "react";
import { useAgentComposer } from "./context";

function synchronizeDropdownViewport(composerElement: HTMLDivElement) {
	// Radix popover wrappers are fixed-positioned, so their
	// inset values need to be in layout-viewport coordinates.
	// The visual viewport can be offset inside the layout
	// viewport when the mobile keyboard is open. Treat
	// `visualViewport.offsetTop` as a clamp only when it yields
	// a positive height, since mobile WebKit can report mixed
	// coordinate systems while the keyboard is settling.
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
}

/** Anchors composer interactions and synchronizes mobile dropdown geometry. */
export function Frame({
	children,
	className,
}: {
	children: React.ReactNode;
	className?: string;
}) {
	const { state, actions, meta } = useAgentComposer();
	const { composerElement, setComposerElement } = meta;
	const [isDragging, setIsDragging] = useState(false);

	useEffect(() => {
		if (!composerElement) {
			return;
		}

		return synchronizeDropdownViewport(composerElement);
	}, [composerElement]);

	const handleKeyDown = (event: React.KeyboardEvent) => {
		if (event.key !== "Escape" || !(event.target instanceof Node)) {
			return;
		}

		// Portaled menus and dialogs own their Escape action, not the draft behind them.
		if (!event.currentTarget.contains(event.target)) {
			return;
		}

		if (state.isEditingHistoryMessage) {
			if (state.isLoading) {
				return;
			}
			event.preventDefault();
			actions.cancelHistoryEdit?.();
		} else if (
			state.isStreaming &&
			actions.interrupt &&
			!state.isInterruptPending
		) {
			event.preventDefault();
			actions.interrupt();
		}
	};

	const handleDragOver = (event: React.DragEvent) => {
		if (!event.dataTransfer.types.includes("Files")) {
			return;
		}

		event.preventDefault();

		if (state.canAttachFiles) {
			setIsDragging(true);
		}
	};

	const handleDragLeave = (event: React.DragEvent) => {
		if (!state.canAttachFiles) {
			return;
		}

		if (
			!(event.relatedTarget instanceof Node) ||
			!event.currentTarget.contains(event.relatedTarget)
		) {
			setIsDragging(false);
		}
	};

	const handleDrop = (event: React.DragEvent) => {
		if (event.dataTransfer.files.length === 0) {
			return;
		}

		event.preventDefault();
		setIsDragging(false);

		if (state.canAttachFiles) {
			actions.attachFiles(Array.from(event.dataTransfer.files));
		}
	};

	return (
		<div
			ref={setComposerElement}
			data-testid="chat-composer"
			className={cn(
				"relative z-10 rounded-2xl bg-surface-secondary sm:bg-surface-secondary/45 p-1 shadow-xs has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40",
				state.needsSetup && "sm:bg-surface-secondary",
				isDragging && "ring-2 ring-content-link/40",
				(state.isEditingHistoryMessage || state.warning) &&
					"shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				className,
			)}
			onKeyDown={handleKeyDown}
			onDragOver={handleDragOver}
			onDragLeave={handleDragLeave}
			onDrop={handleDrop}
		>
			{children}
		</div>
	);
}
