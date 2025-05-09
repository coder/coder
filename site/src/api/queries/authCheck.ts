import { API } from "api/api";
import type { AuthorizationRequest } from "api/typesGenerated";

const AUTHORIZATION_KEY = "authorization";

export const getAuthorizationKey = (req: AuthorizationRequest) =>
	[AUTHORIZATION_KEY, req] as const;

export const checkAuthorization = (req: AuthorizationRequest) => {
	return {
		queryKey: getAuthorizationKey(req),
		queryFn: () => API.checkAuthorization(req),
	};
};
