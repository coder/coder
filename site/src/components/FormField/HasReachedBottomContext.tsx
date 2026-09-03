import {
	createContext,
	type FC,
	type PropsWithChildren,
	useEffect,
	useState,
} from "react";

/**
 * Tracks whether the user has scrolled to the bottom of the window at least
 * once. Consumed by required-field wrappers (via `useContext`) so they can flip
 * empty required fields to the destructive red outline once the user reaches
 * the end of the page, catching required fields that were visible but never
 * scrolled past. Sticky: stays true once triggered.
 *
 * This is a context + provider component, not a custom hook: consumers read it
 * with `useContext(HasReachedBottomContext)` directly.
 */
export const HasReachedBottomContext = createContext<{
	hasReachedBottom: boolean;
}>({ hasReachedBottom: false });

const BOTTOM_TOLERANCE_PX = 4;

export const HasReachedBottomProvider: FC<PropsWithChildren> = ({
	children,
}) => {
	const [hasReachedBottom, setHasReachedBottom] = useState(false);

	useEffect(() => {
		if (hasReachedBottom) {
			return;
		}
		const check = () => {
			const scrolled = window.scrollY + window.innerHeight;
			const total = document.documentElement.scrollHeight;
			if (scrolled >= total - BOTTOM_TOLERANCE_PX) {
				setHasReachedBottom(true);
			}
		};
		check();
		window.addEventListener("scroll", check, { passive: true });
		window.addEventListener("resize", check);
		return () => {
			window.removeEventListener("scroll", check);
			window.removeEventListener("resize", check);
		};
	}, [hasReachedBottom]);

	return (
		<HasReachedBottomContext.Provider value={{ hasReachedBottom }}>
			{children}
		</HasReachedBottomContext.Provider>
	);
};
