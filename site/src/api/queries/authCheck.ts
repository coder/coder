import { API } from "#/api/api";
import type {
	AuthorizationRequest,
	AuthorizationResponse,
} from "#/api/typesGenerated";
import type { MetadataState, MetadataValue } from "#/hooks/useEmbeddedMetadata";
import { disabledRefetchOptions } from "./util";

const AUTHORIZATION_KEY = "authorization";

export const getAuthorizationKey = (req: AuthorizationRequest) =>
	[AUTHORIZATION_KEY, req] as const;

export function checkAuthorization<TResponse extends AuthorizationResponse>(
	req: AuthorizationRequest,
	metadata?: MetadataState<TResponse & MetadataValue>,
) {
	const base = {
		queryKey: getAuthorizationKey(req),
		queryFn: () => API.checkAuthorization<TResponse>(req),
	};

	if (metadata?.available) {
		return {
			...base,
			initialData: metadata.value as TResponse,
			...disabledRefetchOptions,
		};
	}
	return base;
}
