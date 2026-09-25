import { useCallback, useState } from "react";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import {
	belowLgViewportMediaQuery,
	belowMdViewportMediaQuery,
} from "#/utils/mobile";

export const EXPANDED_WIDTH = 240;
// Twice the 32px icon center offset, so rail icons are centered.
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
		// Storage may be unavailable.
	}
}

type UseSidebarResizeReturn = {
	width: number;
	collapsed: boolean;
	/** Below md, where the expanded state is a drawer. */
	mobile: boolean;
	expand: () => void;
	collapse: () => void;
	toggle: () => void;
};

/** Collapsed state persisted per `storageKey`. Below lg it starts collapsed. */
export function useSidebarResize(storageKey: string): UseSidebarResizeReturn {
	const isNarrowViewport = useMediaQuery(belowLgViewportMediaQuery);
	const mobile = useMediaQuery(belowMdViewportMediaQuery);

	const [collapsed, setCollapsed] = useState(
		() => isNarrowViewport || readCollapsed(storageKey),
	);

	// Reset to the default for the viewport when crossing lg or md.
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

	// The mobile drawer state is not persisted.
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
