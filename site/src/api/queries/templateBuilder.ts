import { API } from "#/api/api";

export const templateBuilderBases = () => ({
	queryKey: ["templateBuilder", "bases"],
	queryFn: API.getTemplateBuilderBases,
});
