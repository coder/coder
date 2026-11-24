import { useEffect, useRef } from "react";

type LatestAbortController = {
	start: () => AbortController;
	clear: (controller: AbortController) => boolean;
	abort: () => void;
};

export const useLatestAbortController = (): LatestAbortController => {
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
