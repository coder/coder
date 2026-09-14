import { API } from "#/api/api";

export const updateCheckQueryKey = ["updateCheck"];

export const updateCheck = () => {
	return {
		queryKey: updateCheckQueryKey,
		queryFn: () => API.getUpdateCheck(),
	};
};
