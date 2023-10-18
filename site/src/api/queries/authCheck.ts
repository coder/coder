import { AuthorizationRequest } from "api/typesGenerated";
import * as API from "api/api";

export const AUTHORIZATION_KEY = "authorization";

export const getAuthorizationKey = (req: AuthorizationRequest) =>
  [AUTHORIZATION_KEY, req] as const;

export const checkAuthorization = (req: AuthorizationRequest) => {
  return {
    queryKey: getAuthorizationKey(req),
    queryFn: () => API.checkAuthorization(req),
  };
};
