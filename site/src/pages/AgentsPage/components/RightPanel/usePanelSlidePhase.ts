import { useState } from "react";

/**
 * Where the side-by-side panel is in its slide. "closing" and "slidingOut"
 * keep the panel laid out until their animation ends; "closed" hides it with
 * display: none so hidden content, such as a terminal, stops rendering.
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
	isDragging: boolean;
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
		// Leaving expanded mode has nothing to animate from.
		return prev.isOpen || next.isDragging ? "open" : "opening";
	}
	if (wasSideBySide && !isSideBySide(next)) {
		// Crossing below lg drops the lg: width, so a keyframe slides it out.
		if (next.isBelowLg && !next.isExpanded) {
			return "slidingOut";
		}
		if (next.isOpen) {
			return "open";
		}
		return next.isDragging ? "closed" : "closing";
	}
	if (prev.isOpen && !next.isOpen && !wasSideBySide && phase !== "slidingOut") {
		return "closed";
	}
	if (phase === "opening" && next.isDragging) {
		// A drag removes the transition, so no transitionend will arrive.
		return "open";
	}
	if (phase === "slidingOut" && !next.isBelowLg) {
		return next.isOpen ? "open" : "closed";
	}
	return phase;
};

/**
 * Tracks the right panel's slide phase and returns the end handlers that
 * advance it once a transition or animation on the panel itself finishes.
 */
export const usePanelSlidePhase = (inputs: PanelSlideInputs) => {
	const [prev, setPrev] = useState(inputs);
	const [phase, setPhase] = useState<PanelSlidePhase>(
		inputs.isOpen ? "open" : "closed",
	);
	const changed =
		prev.isOpen !== inputs.isOpen ||
		prev.isExpanded !== inputs.isExpanded ||
		prev.isBelowLg !== inputs.isBelowLg ||
		prev.isDragging !== inputs.isDragging;
	let currentPhase = phase;
	if (changed) {
		currentPhase = nextPanelSlidePhase(phase, prev, inputs);
		setPrev(inputs);
		setPhase(currentPhase);
	}

	const onTransitionEnd = (e: React.TransitionEvent) => {
		if (e.target !== e.currentTarget || e.propertyName !== "width") {
			return;
		}
		setPhase((p) =>
			p === "opening" ? "open" : p === "closing" ? "closed" : p,
		);
	};
	const onAnimationEnd = (e: React.AnimationEvent) => {
		if (e.target === e.currentTarget) {
			setPhase((p) => (p === "slidingOut" ? "closed" : p));
		}
	};

	return { phase: currentPhase, onTransitionEnd, onAnimationEnd };
};
