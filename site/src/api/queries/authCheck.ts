import { client } from "api/api";
import type { AuthorizationRequest } from "api/typesGenerated";

export const AUTHORIZATION_KEY = "authorization";

export const getAuthorizationKey = (req: AuthorizationRequest) =>
  [AUTHORIZATION_KEY, req] as const;

export const checkAuthorization = (req: AuthorizationRequest) => {
  return {
    queryKey: getAuthorizationKey(req),
    queryFn: () => client.api.checkAuthorization(req),
  };
};
