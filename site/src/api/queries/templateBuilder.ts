import { API } from "#/api/api";

export const templateBuilderBases = () => ({
	queryKey: ["templateBuilder", "bases"],
	queryFn: API.getTemplateBuilderBases,
});

export const templateBuilderModules = (base?: string) => ({
	queryKey: ["templateBuilder", "modules", base ?? ""],
	queryFn: () => API.getTemplateBuilderModules(base),
	staleTime: Number.POSITIVE_INFINITY,
});

export const createTemplateFromBuilder = () => ({
	mutationFn: API.createTemplateFromBuilder,
});
