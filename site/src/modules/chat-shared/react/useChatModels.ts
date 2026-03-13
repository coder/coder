import { useCallback, useEffect, useRef, useState } from "react";
import type { ChatModelOption } from "../core";
import { useChatRuntimeContext } from "./ChatRuntimeProvider";

/** @public Result state for the shared chat models hook. */
export type UseChatModelsResult = {
	models: readonly ChatModelOption[];
	isLoading: boolean;
	error: unknown;
	refetch: () => Promise<void>;
};

export const useChatModels = (): UseChatModelsResult => {
	const { runtime, modelCache } = useChatRuntimeContext();
	const isMountedRef = useRef(true);
	const [models, setModels] = useState<readonly ChatModelOption[]>(
		() => modelCache.current?.models ?? [],
	);
	const [isLoading, setIsLoading] = useState(
		() => modelCache.current?.status === "loading",
	);
	const [error, setError] = useState<unknown>(
		() => modelCache.current?.error ?? null,
	);

	const loadModels = useCallback(
		(
			forceRefresh: boolean,
		): {
			requestID: number;
			promise: Promise<readonly ChatModelOption[]> | null;
			models: readonly ChatModelOption[];
			error: unknown;
			status: "loading" | "success" | "error";
		} => {
			const previousRequestID = modelCache.current?.requestID ?? 0;
			if (forceRefresh) {
				modelCache.current = null;
			}

			const existingCache = modelCache.current;
			if (existingCache) {
				return existingCache;
			}

			const requestID = previousRequestID + 1;
			const nextCache = {
				requestID,
				status: "loading" as const,
				promise: runtime.listModels(),
				models: [] as readonly ChatModelOption[],
				error: null,
			};
			modelCache.current = nextCache;
			nextCache.promise
				.then((resolvedModels) => {
					if (modelCache.current?.requestID !== requestID) {
						return;
					}
					modelCache.current = {
						requestID,
						status: "success",
						promise: null,
						models: resolvedModels,
						error: null,
					};
				})
				.catch((loadError) => {
					if (modelCache.current?.requestID !== requestID) {
						return;
					}
					modelCache.current = {
						requestID,
						status: "error",
						promise: null,
						models: [],
						error: loadError,
					};
				});
			return nextCache;
		},
		[modelCache, runtime],
	);

	const applyCacheState = useCallback(
		(requestID: number): void => {
			if (
				!isMountedRef.current ||
				modelCache.current?.requestID !== requestID
			) {
				return;
			}
			setModels(modelCache.current.models);
			setError(modelCache.current.error);
			setIsLoading(modelCache.current.status === "loading");
		},
		[modelCache],
	);

	const awaitCacheEntry = useCallback(
		async (cacheEntry: ReturnType<typeof loadModels>): Promise<void> => {
			if (!cacheEntry.promise) {
				await Promise.resolve();
				applyCacheState(cacheEntry.requestID);
				return;
			}
			try {
				await cacheEntry.promise;
			} catch {
				// The shared cache stores the rejected state for the hook to read.
			}
			await Promise.resolve();
			applyCacheState(cacheEntry.requestID);
		},
		[applyCacheState],
	);

	useEffect(() => {
		isMountedRef.current = true;
		const cacheEntry = loadModels(false);
		setModels(cacheEntry.models);
		setIsLoading(cacheEntry.status === "loading");
		setError(cacheEntry.error);
		void awaitCacheEntry(cacheEntry);
		return () => {
			isMountedRef.current = false;
		};
	}, [awaitCacheEntry, loadModels]);

	const refetch = useCallback(async (): Promise<void> => {
		const cacheEntry = loadModels(true);
		setModels(cacheEntry.models);
		setIsLoading(cacheEntry.status === "loading");
		setError(cacheEntry.error);
		await awaitCacheEntry(cacheEntry);
	}, [awaitCacheEntry, loadModels]);

	return { models, isLoading, error, refetch };
};
