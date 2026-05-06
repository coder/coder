import {
	createContext,
	createElement,
	type FC,
	type ReactNode,
	useContext,
	useEffect,
	useState,
} from "react";

export const SIDEBAR_MIN_WIDTH = 200;
const SIDEBAR_DEFAULT_WIDTH = 320;
export const SIDEBAR_MAX_WIDTH_RATIO = 0.4;
const SIDEBAR_NARROW_THRESHOLD = 600;
const SIDEBAR_LARGE_THRESHOLD = 800;

const STORAGE_KEY = "agents.sidebar-width";

function getMaxWidth(): number {
	return Math.max(
		SIDEBAR_MIN_WIDTH,
		Math.floor(window.innerWidth * SIDEBAR_MAX_WIDTH_RATIO),
	);
}

function loadPersistedWidth(): number {
	const stored = localStorage.getItem(STORAGE_KEY);
	if (!stored) {
		return SIDEBAR_DEFAULT_WIDTH;
	}
	const parsed = Number.parseInt(stored, 10);
	if (
		Number.isNaN(parsed) ||
		parsed < SIDEBAR_MIN_WIDTH ||
		parsed > getMaxWidth()
	) {
		return SIDEBAR_DEFAULT_WIDTH;
	}
	return parsed;
}

type Layout = "narrow" | "medium" | "large";

interface SidebarWidthContextValue {
	width: number;
	layout: Layout;
	setWidth: (w: number) => void;
}

const SidebarWidthContext = createContext<SidebarWidthContextValue | null>(
	null,
);

export const SidebarWidthProvider: FC<{ children: ReactNode }> = ({
	children,
}) => {
	const [width, setWidth] = useState(loadPersistedWidth);

	// Clamp width when the viewport shrinks so the sidebar never
	// exceeds its allowed proportion of the screen.
	useEffect(() => {
		const handleResize = () => {
			setWidth((prev) => Math.min(prev, getMaxWidth()));
		};
		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, []);

	useEffect(() => {
		localStorage.setItem(STORAGE_KEY, String(width));
	}, [width]);

	const layout: Layout =
		width < SIDEBAR_NARROW_THRESHOLD
			? "narrow"
			: width >= SIDEBAR_LARGE_THRESHOLD
				? "large"
				: "medium";

	const value: SidebarWidthContextValue = { width, layout, setWidth };

	return createElement(SidebarWidthContext.Provider, { value }, children);
};

export function useSidebarWidth(): SidebarWidthContextValue {
	const ctx = useContext(SidebarWidthContext);
	if (!ctx) {
		throw new Error(
			"useSidebarWidth must be used within a SidebarWidthProvider",
		);
	}
	return ctx;
}
