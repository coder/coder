import type { FileDiffMetadata } from "@pierre/diffs";
import { type RefObject, useEffect, useRef, useState } from "react";

interface UseActiveFileTrackingOptions {
	viewportRef: RefObject<HTMLElement | null>;
	sortedFiles: readonly FileDiffMetadata[];
	enabled: boolean;
	scrollToFile?: string | null;
	onScrollComplete?: () => void;
}

interface UseActiveFileTrackingReturn {
	treeActiveFile: string | null;
	setFileRef: (name: string, el: HTMLDivElement | null) => void;
	handleFileClick: (name: string) => void;
}

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

	const [viewportHeight, setViewportHeight] = useState(0);

	// viewportRef is a stable RefObject whose identity never changes, so
	// an effect that depends on it won't re-run when .current transitions
	// from null to the actual DOM node (e.g. after a loading state).
	// Keep a state mirror that flips exactly once when the element mounts.
	const [viewportEl, setViewportEl] = useState<HTMLElement | null>(null);
	useEffect(() => {
		setViewportEl(viewportRef.current);
	});

	useEffect(() => {
		if (!viewportEl) return;
		setViewportHeight(viewportEl.clientHeight);
		const ro = new ResizeObserver(([entry]) => {
			setViewportHeight(Math.round(entry.contentRect.height));
		});
		ro.observe(viewportEl);
		return () => ro.disconnect();
	}, [viewportEl]);

	const sortedFilesRef = useRef(sortedFiles);
	useEffect(() => {
		sortedFilesRef.current = sortedFiles;
	});

	const fileListKey = sortedFiles.map((f) => f.name).join("\0");

	const setFileRef = (name: string, el: HTMLDivElement | null) => {
		if (el) {
			fileRefs.current.set(name, el);
		} else {
			fileRefs.current.delete(name);
		}
	};

	useEffect(() => {
		if (!enabled || fileListKey === "" || viewportHeight === 0) return;
		if (!viewportEl) return;

		const bottomMargin = Math.round(viewportHeight * 0.95);

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
				for (const file of sortedFilesRef.current) {
					if (intersecting.has(file.name)) {
						activeFileRef.current = file.name;
						setTreeActiveFile(file.name);
						break;
					}
				}
			},
			{
				root: viewportEl,
				// Observe only the top ~5% strip of the viewport height.
				rootMargin: `0px 0px -${bottomMargin}px 0px`,
				threshold: 0,
			},
		);

		for (const [, el] of fileRefs.current.entries()) {
			observer.observe(el);
		}

		return () => observer.disconnect();
	}, [enabled, fileListKey, viewportEl, viewportHeight]);

	const handleFileClick = (name: string) => {
		const el = fileRefs.current.get(name);
		if (el) {
			el.scrollIntoView({ block: "start" });
			activeFileRef.current = name;
			setTreeActiveFile(name);
		}
	};

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
