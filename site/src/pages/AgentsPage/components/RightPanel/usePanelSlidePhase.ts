import { useLayoutEffect, useState } from "react";

/**
 * Where the side-by-side panel is in its slide. "opening", "closing" (a
 * width transition), and "slidingOut" (a keyframe animation) last until the
 * panel's running animations settle. "closed" means no slide is running;
 * RightPanel decides what to hide from the phase and whether it is open.
 */
export type PanelSlidePhase =
	| "opening"
	| "open"
	| "closing"
	| "slidingOut"
	| "closed";

type PanelSlideInputs = {
	isOpen: boolean;
	isExpanded: boolean;
	isBelowLg: boolean;
	isPointerResizing: boolean;
};

const isSideBySide = ({ isOpen, isExpanded, isBelowLg }: PanelSlideInputs) =>
	isOpen && !isExpanded && !isBelowLg;

/** The phase after the panel inputs change from prev to next. */
export const nextPanelSlidePhase = (
	phase: PanelSlidePhase,
	prev: PanelSlideInputs,
	next: PanelSlideInputs,
): PanelSlidePhase => {
	const wasSideBySide = isSideBySide(prev);
	if (isSideBySide(next) && !wasSideBySide) {
		// Only a closed panel slides open. Leaving expanded mode or widening
		// past lg has no start width to animate from, and a drag sets the
		// width directly.
		return prev.isOpen || next.isPointerResizing ? "open" : "opening";
	}
	if (wasSideBySide && !isSideBySide(next)) {
		// Crossing below lg drops the lg: width, so a keyframe slides it out.
		if (next.isBelowLg && !next.isExpanded) {
			return "slidingOut";
		}
		if (next.isOpen) {
			return "open";
		}
		return next.isPointerResizing ? "closed" : "closing";
	}
	if (!prev.isOpen && next.isOpen) {
		// Opening the overlay or expanded panel ends any slide.
		return "open";
	}
	if (prev.isOpen && !next.isOpen && phase !== "slidingOut") {
		// The page closes the panel a render after the lg crossing, so a
		// slide-out keeps running through that close.
		return "closed";
	}
	return phase;
};

const settledPhase = (phase: PanelSlidePhase): PanelSlidePhase =>
	phase === "opening" ? "open" : "closed";

/**
 * Tracks the right panel's slide phase. A slide ends when the panel's own
 * animations settle, or at once when none are running, so a transition that
 * never starts (a class change, a missing browser feature) cannot leave the
 * phase stuck.
 */
export const usePanelSlidePhase = (
	panelRef: React.RefObject<HTMLElement | null>,
	inputs: PanelSlideInputs,
): PanelSlidePhase => {
	const [prev, setPrev] = useState(inputs);
	const [phase, setPhase] = useState<PanelSlidePhase>(
		inputs.isOpen ? "open" : "closed",
	);
	const changed =
		prev.isOpen !== inputs.isOpen ||
		prev.isExpanded !== inputs.isExpanded ||
		prev.isBelowLg !== inputs.isBelowLg ||
		prev.isPointerResizing !== inputs.isPointerResizing;
	let currentPhase = phase;
	if (changed) {
		currentPhase = nextPanelSlidePhase(phase, prev, inputs);
		setPrev(inputs);
		setPhase(currentPhase);
	}

	const { isOpen } = inputs;
	useLayoutEffect(() => {
		// A slide-out waits for the close that applies its animation.
		if (
			phase === "open" ||
			phase === "closed" ||
			(phase === "slidingOut" && isOpen)
		) {
			return;
		}
		const settle = () => setPhase(settledPhase(phase));
		const animations = panelRef.current?.getAnimations?.() ?? [];
		if (animations.length === 0) {
			settle();
			return;
		}
		let cancelled = false;
		void Promise.allSettled(animations.map((a) => a.finished)).then(() => {
			if (!cancelled) {
				settle();
			}
		});
		return () => {
			cancelled = true;
		};
	}, [phase, isOpen, panelRef]);

	return currentPhase;
};
