import type { UseQueryOptions } from "react-query";
import { API } from "#/api/api";
import { type Experiment, Experiments } from "#/api/typesGenerated";
import type { RuntimeHtmlMetadata } from "#/hooks/useEmbeddedMetadata";

export const experimentsKey = (userId: string) =>
	["experiments", userId] as const;

// Experiments are decided per user and can change at runtime. Backend
// gates stay authoritative; this only bounds how long the UI shows an
// outdated list.
const experimentsStaleTime = 60_000;

export const experiments = (
	userId: string,
	metadata: Pick<RuntimeHtmlMetadata, "user" | "experiments">,
) => {
	// The embedded list was rendered for the user who loaded the page, so
	// only that user may use it, and it is as old as the page.
	const useEmbedded =
		metadata.experiments.available &&
		metadata.user.available &&
		metadata.user.value?.id === userId;

	return {
		queryKey: experimentsKey(userId),
		queryFn: () => API.getExperiments(),
		initialData: useEmbedded ? metadata.experiments.value : undefined,
		initialDataUpdatedAt: useEmbedded ? performance.timeOrigin : undefined,
		staleTime: experimentsStaleTime,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	} satisfies UseQueryOptions<Experiment[]>;
};

export const availableExperiments = () => {
	return {
		queryKey: ["availableExperiments"],
		queryFn: async () => API.getAvailableExperiments(),
	};
};

export const isKnownExperiment = (experiment: string): boolean => {
	return Experiments.includes(experiment as Experiment);
};
