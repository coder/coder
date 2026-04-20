import type { FileDiffMetadata } from "@pierre/diffs";
import { type RefObject, useEffect, useRef, useState } from "react";

interface UseActiveFileTrackingOptions {
	/** Ref to the scrollable diff viewport element. */
	viewportRef: RefObject<HTMLElement | null>;
	/** Sorted file diffs (determines document order for picking the
	 *  first intersecting file). */
	sortedFiles: readonly FileDiffMetadata[];
	/** When false the IntersectionObserver is not set up (e.g. when the
	 *  file tree sidebar is hidden). */
	enabled: boolean;
	/** When set, scroll to the named file and call `onScrollComplete`
	 *  afterwards. The parent should reset to null after completion. */
	scrollToFile?: string | null;
	/** Called after a `scrollToFile` request has been processed. */
	onScrollComplete?: () => void;
}

interface UseActiveFileTrackingReturn {
	/** State value for the tree sidebar highlight. */
	treeActiveFile: string | null;
	/** Ref callback to register/unregister a file wrapper element. */
	setFileRef: (name: string, el: HTMLDivElement | null) => void;
	/** Click handler for the file tree – scrolls to the file and
	 *  updates the active highlight. */
	handleFileClick: (name: string) => void;
}

/**
 * Tracks which file is currently at the top of the diff viewport using
 * an `IntersectionObserver` and exposes helpers for programmatic
 * scroll-to-file.
 *
 * Extracted from `DiffViewer` to keep the main component focused on
 * layout and rendering.
 */
export function useActiveFileTracking({
	viewportRef,
	sortedFiles,
	enabled,
	scrollToFile,
	onScrollComplete,
}: UseActiveFileTrackingOptions): UseActiveFileTrackingReturn {
	const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map());
	const activeFileRef = useRef<string | null>(null);
	const [treeActiveFile, setTreeActiveFile] = useState<string | null>(null);

	// Track viewport height so the IntersectionObserver's rootMargin
	// adapts when the diff panel is resized (window resize, layout
	// split, etc.). Without this the observation strip computed at
	// setup time becomes stale after a resize.
	const [viewportHeight, setViewportHeight] = useState(0);
	useEffect(() => {
		const viewport = viewportRef.current;
		if (!viewport) return;
		setViewportHeight(viewport.clientHeight);
		const ro = new ResizeObserver(([entry]) => {
			setViewportHeight(Math.round(entry.contentRect.height));
		});
		ro.observe(viewport);
		return () => ro.disconnect();
	}, [viewportRef]);

	// Keep sortedFiles in a ref so the IntersectionObserver callback
	// always reads the latest value without needing sortedFiles in
	// the effect's dependency array.
	const sortedFilesRef = useRef(sortedFiles);
	useEffect(() => {
		sortedFilesRef.current = sortedFiles;
	});

	// Stable identity: only changes when the actual file list changes,
	// not on every render (sortedFiles is rebuilt inline by the caller).
	const fileListKey = sortedFiles.map((f) => f.name).join("\0");

	// Ref callback that sets up per-file refs.
	const setFileRef = (name: string, el: HTMLDivElement | null) => {
		if (el) {
			fileRefs.current.set(name, el);
		} else {
			fileRefs.current.delete(name);
		}
	};

	// Track which file is at the top of the diff scroll area using
	// an IntersectionObserver. The observer watches a narrow strip
	// at the top 5% of the viewport. When a file boundary crosses
	// this strip, the callback fires — zero synchronous layout
	// reads per frame.
	useEffect(() => {
		if (!enabled || fileListKey === "" || viewportHeight === 0) return;
		const viewport = viewportRef.current;
		if (!viewport) return;

		// Percentage rootMargin values resolve against the root's width
		// (per CSS spec), not its height. Compute an explicit pixel value
		// from the viewport height so the observation strip is reliably
		// 5% of the visible height regardless of aspect ratio.
		const bottomMargin = Math.round(viewportHeight * 0.95);

		// Track which files currently intersect the top strip.
		const intersecting = new Set<string>();

		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					const name = (entry.target as HTMLElement).dataset.fileName;
					if (!name) continue;
					if (entry.isIntersecting) {
						intersecting.add(name);
					} else {
						intersecting.delete(name);
					}
				}
				// Pick the first intersecting file in document order.
				for (const file of sortedFilesRef.current) {
					if (intersecting.has(file.name)) {
						activeFileRef.current = file.name;
						setTreeActiveFile(file.name);
						break;
					}
				}
			},
			{
				root: viewport,
				// Observe only the top ~5% strip of the viewport height.
				rootMargin: `0px 0px -${bottomMargin}px 0px`,
				threshold: 0,
			},
		);

		for (const [, el] of fileRefs.current.entries()) {
			observer.observe(el);
		}

		return () => observer.disconnect();
	}, [enabled, fileListKey, viewportRef, viewportHeight]);

	const handleFileClick = (name: string) => {
		const el = fileRefs.current.get(name);
		if (el) {
			el.scrollIntoView({ block: "start" });
			activeFileRef.current = name;
			setTreeActiveFile(name);
		}
	};

	// Scroll to a file programmatically when the parent sets
	// scrollToFile. This enables external navigation (e.g. clicking
	// a file reference chip in the chat input).
	useEffect(() => {
		if (scrollToFile) {
			const el = fileRefs.current.get(scrollToFile);
			if (el) {
				el.scrollIntoView({ block: "start", behavior: "instant" });
				activeFileRef.current = scrollToFile;
				setTreeActiveFile(scrollToFile);
			}
			onScrollComplete?.();
		}
	}, [scrollToFile, onScrollComplete]);

	return {
		treeActiveFile,
		setFileRef,
		handleFileClick,
	};
}
