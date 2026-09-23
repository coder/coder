import { useCallback, useState } from "react";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import {
	belowLgViewportMediaQuery,
	belowMdViewportMediaQuery,
} from "#/utils/mobile";

export const EXPANDED_WIDTH = 240;
// Icon center sits at nav-pl(12) + btn-px(12) + icon/2(8) = 32px.
// Double that so the icon is horizontally centered when collapsed.
export const COLLAPSED_WIDTH = 64;

function readCollapsed(key: string): boolean {
	try {
		return localStorage.getItem(key) === "collapsed";
	} catch {
		return false;
	}
}

function persistCollapsed(key: string, collapsed: boolean): void {
	try {
		localStorage.setItem(key, collapsed ? "collapsed" : "expanded");
	} catch {
		// Silently ignore write failures.
	}
}

type UseSidebarResizeReturn = {
	width: number;
	collapsed: boolean;
	/**
	 * Below the md breakpoint the expanded sidebar is a full-width drawer
	 * over the content rather than a column beside it.
	 */
	mobile: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
	/** Force the sidebar to collapse. */
	collapse: () => void;
	/** Toggle collapsed/expanded state. */
	toggle: () => void;
};

/**
 * Two-state sidebar width persisted per `storageKey`. Two environmental
 * rules override the persisted choice without being persisted: the
 * sidebar starts and stays collapsed below the lg breakpoint, and below
 * md the expanded state is a drawer that never outlives the viewport
 * that opened it.
 */
export function useSidebarResize(storageKey: string): UseSidebarResizeReturn {
	const isNarrowViewport = useMediaQuery(belowLgViewportMediaQuery);
	const mobile = useMediaQuery(belowMdViewportMediaQuery);

	// Start collapsed on narrow viewports regardless of the persisted
	// preference, so page content is not cut off on load.
	const [collapsed, setCollapsed] = useState(
		() => isNarrowViewport || readCollapsed(storageKey),
	);

	// Crossing the lg or md breakpoint in either direction recomputes the
	// environmental state: shrinking collapses the column or closes the
	// drawer, growing restores the persisted preference. The crossing is
	// detected against the previous render's values, so the reset happens
	// in render rather than an effect.
	const [prevViewport, setPrevViewport] = useState({
		narrow: isNarrowViewport,
		mobile,
	});
	if (
		prevViewport.narrow !== isNarrowViewport ||
		prevViewport.mobile !== mobile
	) {
		setPrevViewport({ narrow: isNarrowViewport, mobile });
		setCollapsed(isNarrowViewport || readCollapsed(storageKey));
	}

	// Mobile drawer state is environmental and never written to storage,
	// so leaving mobile restores the desktop preference.
	const setCollapsedAndPersist = useCallback(
		(next: boolean) => {
			setCollapsed(next);
			if (!mobile) {
				persistCollapsed(storageKey, next);
			}
		},
		[mobile, storageKey],
	);

	const expand = useCallback(
		() => setCollapsedAndPersist(false),
		[setCollapsedAndPersist],
	);
	const collapse = useCallback(
		() => setCollapsedAndPersist(true),
		[setCollapsedAndPersist],
	);
	const toggle = useCallback(
		() => setCollapsedAndPersist(!collapsed),
		[setCollapsedAndPersist, collapsed],
	);

	return {
		width: collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH,
		collapsed,
		mobile,
		expand,
		collapse,
		toggle,
	};
}
