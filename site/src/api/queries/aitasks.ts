import type { UseQueryOptions } from "react-query";
import { API } from "../api";
import type * as TypesGen from "../typesGenerated";

export const aiTasksStats = (): UseQueryOptions<TypesGen.AITasksStatsResponse> => ({
	queryKey: ["aitasks", "stats"],
	queryFn: API.getAITasksStats,
});
