import {
	type WorkerInitializationRenderOptions,
	WorkerPoolContextProvider,
	type WorkerPoolOptions,
} from "@pierre/diffs/react";
import type { FC, ReactNode } from "react";

interface DiffsWorkerPoolProviderProps {
	children: ReactNode;
}

const highlighterOptions: WorkerInitializationRenderOptions = {
	theme: {
		dark: "github-dark-high-contrast",
		light: "github-light",
	},
};

const getPoolSize = (): number => {
	const cores = globalThis.navigator?.hardwareConcurrency ?? 2;
	// From Kyle: This is just arbitrarily chosen by me.
	return Math.min(Math.max(1, cores - 1), 3);
};

const hasWorkerSupport = (): boolean =>
	typeof window !== "undefined" && typeof Worker !== "undefined";

export const DiffsWorkerPoolProvider: FC<DiffsWorkerPoolProviderProps> = ({
	children,
}) => {
	if (!hasWorkerSupport()) {
		return <>{children}</>;
	}

	const poolOptions: WorkerPoolOptions = {
		poolSize: getPoolSize(),
		workerFactory: () =>
			new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), {
				type: "module",
			}),
	};

	return (
		<WorkerPoolContextProvider
			poolOptions={poolOptions}
			highlighterOptions={highlighterOptions}
		>
			{children}
		</WorkerPoolContextProvider>
	);
};
