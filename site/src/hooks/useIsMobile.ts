import { useEffect, useState } from "react";

const MOBILE_QUERY = "(max-width: 767px)";

export function useIsMobile(): boolean {
	const [isMobile, setIsMobile] = useState(
		() => window.matchMedia(MOBILE_QUERY).matches,
	);

	useEffect(() => {
		const mq = window.matchMedia(MOBILE_QUERY);
		const handler = (e: MediaQueryListEvent) => setIsMobile(e.matches);
		mq.addEventListener("change", handler);
		return () => mq.removeEventListener("change", handler);
	}, []);

	return isMobile;
}
