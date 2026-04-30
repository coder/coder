import { useEffect, useRef } from "react";

type LatestAbortController = {
	start: () => AbortController;
	clear: (controller: AbortController) => boolean;
	abort: () => void;
};

export const useLatestAbortController = (
	shouldAbortCurrentRequest = false,
): LatestAbortController => {
	const controllerRef = useRef<AbortController | null>(null);

	const abort = () => {
		controllerRef.current?.abort();
		controllerRef.current = null;
	};

	useEffect(() => {
		return () => {
			controllerRef.current?.abort();
			controllerRef.current = null;
		};
	}, []);

	useEffect(() => {
		if (shouldAbortCurrentRequest) {
			controllerRef.current?.abort();
			controllerRef.current = null;
		}
	}, [shouldAbortCurrentRequest]);

	return {
		start: () => {
			abort();
			const controller = new AbortController();
			controllerRef.current = controller;
			return controller;
		},
		clear: (controller) => {
			if (controllerRef.current !== controller) {
				return false;
			}
			controllerRef.current = null;
			return true;
		},
		abort,
	};
};
