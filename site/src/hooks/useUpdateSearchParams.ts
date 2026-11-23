import { useSearchParams } from "react-router";

/**
 * Returns a function that updates a copy of the latest search params and
 * replaces the current history entry with them.
 */
export const useUpdateSearchParams = () => {
	const [, setSearchParams] = useSearchParams();
	return (update: (params: URLSearchParams) => void) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				update(next);
				return next;
			},
			{ replace: true },
		);
	};
};
