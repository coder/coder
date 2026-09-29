import { describe, expect, it } from "vitest";
import {
	nextPanelSlidePhase,
	type PanelSlidePhase,
} from "./usePanelSlidePhase";

const closed = {
	isOpen: false,
	isExpanded: false,
	isBelowLg: false,
	isDragging: false,
};
const open = { ...closed, isOpen: true };
const expanded = { ...open, isExpanded: true };
const belowLg = { ...closed, isBelowLg: true };

describe("nextPanelSlidePhase", () => {
	it.each<{
		name: string;
		phase: PanelSlidePhase;
		prev: typeof closed;
		next: typeof closed;
		expected: PanelSlidePhase;
	}>([
		{
			name: "opens with a slide",
			phase: "closed",
			prev: closed,
			next: open,
			expected: "opening",
		},
		{
			name: "closes with a slide",
			phase: "open",
			prev: open,
			next: closed,
			expected: "closing",
		},
		{
			name: "slides out when crossing below lg",
			phase: "open",
			prev: open,
			next: belowLg,
			expected: "slidingOut",
		},
		{
			name: "keeps sliding out when the close arrives a render later",
			phase: "slidingOut",
			prev: { ...open, isBelowLg: true },
			next: belowLg,
			expected: "slidingOut",
		},
		{
			name: "opens without a slide when leaving expanded mode",
			phase: "open",
			prev: expanded,
			next: open,
			expected: "open",
		},
		{
			name: "closes an expanded panel without a slide",
			phase: "open",
			prev: expanded,
			next: closed,
			expected: "closed",
		},
		{
			name: "ends an open slide when a drag starts",
			phase: "opening",
			prev: open,
			next: { ...open, isDragging: true },
			expected: "open",
		},
		{
			name: "closes without a slide when dragged shut",
			phase: "open",
			prev: { ...open, isDragging: true },
			next: { ...closed, isDragging: true },
			expected: "closed",
		},
	])("$name", ({ phase, prev, next, expected }) => {
		expect(nextPanelSlidePhase(phase, prev, next)).toBe(expected);
	});
});
