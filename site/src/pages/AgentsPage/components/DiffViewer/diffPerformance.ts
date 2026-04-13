import type { FileDiffMetadata } from "@pierre/diffs";
import userAgentParser from "ua-parser-js";

const LARGE_DIFF_FILE_THRESHOLD = 24;
const LARGE_DIFF_LINE_THRESHOLD = 1500;

const STANDARD_VIRTUALIZER_CONFIG = {
	overscrollSize: 600,
	intersectionObserverMargin: 1600,
} as const;

const SIMPLIFIED_VIRTUALIZER_CONFIG = {
	overscrollSize: 300,
	intersectionObserverMargin: 900,
} as const;

const STANDARD_LAZY_ROOT_MARGIN = "75% 0px";
const SIMPLIFIED_LAZY_ROOT_MARGIN = "50% 0px";

interface DiffRenderMode {
	totalFiles: number;
	totalLines: number;
	isSafari: boolean;
	isLargeDiff: boolean;
	overflow: "wrap" | "scroll";
	showStickyHeaders: boolean;
	enableTreeSync: boolean;
	scrollBehavior: ScrollBehavior;
	lazyMountRootMargin: string;
	virtualizerConfig: {
		overscrollSize: number;
		intersectionObserverMargin: number;
	};
}

export function isSafariUserAgent(userAgent: string): boolean {
	if (!userAgent) {
		return false;
	}

	const { browser, engine, os } = userAgentParser(userAgent);
	if (browser.name === "Safari" || browser.name === "Mobile Safari") {
		return true;
	}

	// All iOS browsers use WebKit, so they share the same scrolling and
	// layout path that regresses on large wrapped diffs.
	return engine.name === "WebKit" && os.name === "iOS";
}

export function getDiffRenderMode(
	parsedFiles: readonly FileDiffMetadata[],
	userAgent = typeof navigator === "undefined" ? "" : navigator.userAgent,
): DiffRenderMode {
	const totalFiles = parsedFiles.length;
	const totalLines = parsedFiles.reduce(
		(lineCount, file) => lineCount + file.unifiedLineCount,
		0,
	);

	const isSafari = isSafariUserAgent(userAgent);
	const isLargeDiff =
		totalFiles >= LARGE_DIFF_FILE_THRESHOLD ||
		totalLines >= LARGE_DIFF_LINE_THRESHOLD;
	const useSimplifiedMode = isSafari || isLargeDiff;

	return {
		totalFiles,
		totalLines,
		isSafari,
		isLargeDiff,
		overflow: useSimplifiedMode ? "scroll" : "wrap",
		showStickyHeaders: !useSimplifiedMode,
		enableTreeSync: !useSimplifiedMode,
		scrollBehavior: useSimplifiedMode ? "instant" : "smooth",
		lazyMountRootMargin: useSimplifiedMode
			? SIMPLIFIED_LAZY_ROOT_MARGIN
			: STANDARD_LAZY_ROOT_MARGIN,
		virtualizerConfig: useSimplifiedMode
			? SIMPLIFIED_VIRTUALIZER_CONFIG
			: STANDARD_VIRTUALIZER_CONFIG,
	};
}
