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
		if (!enabled || sortedFiles.length === 0) return;
		const viewport = viewportRef.current;
		if (!viewport) return;

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
				for (const file of sortedFiles) {
					if (intersecting.has(file.name)) {
						activeFileRef.current = file.name;
						setTreeActiveFile(file.name);
						break;
					}
				}
			},
			{
				root: viewport,
				// Observe only the top ~5% strip of the viewport.
				rootMargin: "0px 0px -95% 0px",
				threshold: 0,
			},
		);

		for (const [, el] of fileRefs.current.entries()) {
			observer.observe(el);
		}

		return () => observer.disconnect();
	}, [enabled, sortedFiles, viewportRef]);

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
