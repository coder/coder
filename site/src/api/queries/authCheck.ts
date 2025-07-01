import { API } from "api/api";
import type {
	AuthorizationRequest,
	AuthorizationResponse,
} from "api/typesGenerated";

const AUTHORIZATION_KEY = "authorization";

export const getAuthorizationKey = (req: AuthorizationRequest) =>
	[AUTHORIZATION_KEY, req] as const;

export const checkAuthorization = <TResponse extends AuthorizationResponse>(
	req: AuthorizationRequest,
) => {
	return {
		queryKey: getAuthorizationKey(req),
		queryFn: () => API.checkAuthorization<TResponse>(req),
	};
};
