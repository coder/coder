import { API } from "api/api";

const getListInterceptionsQueryKey = () => ["aiBridgeInterceptions"];

export const listInterceptions = () => {
	return {
		queryKey: getListInterceptionsQueryKey(),
		queryFn: () => API.experimental.getAIBridgeInterceptions(),
	};
};
