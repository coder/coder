import { useEffect, useRef, useState } from "react";

/**
 * Reports whether the element's content is wider than the element, tracking
 * both container resizes and content changes identified by `contentKey`.
 */
export function useOverflows(contentKey: string) {
	const ref = useRef<HTMLDivElement>(null);
	const [overflows, setOverflows] = useState(false);

	useEffect(() => {
		const element = ref.current;
		if (!element) {
			return;
		}
		const measure = () => {
			setOverflows(element.scrollWidth > element.clientWidth + 1);
		};
		measure();
		const resizeObserver = new ResizeObserver(measure);
		resizeObserver.observe(element);
		return () => resizeObserver.disconnect();
	}, [contentKey]);

	return { ref, overflows };
}
