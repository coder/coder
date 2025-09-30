import { API } from "api/api";
import type { APIKeyWithOwner, TokensFilter } from "api/typesGenerated";
import type { QueryOptions } from "react-query";

export const tokens = (filter: TokensFilter) => {
	return {
		queryKey: ["tokens", filter.include_all],
		queryFn: () => API.getTokens(filter),
	} satisfies QueryOptions<APIKeyWithOwner[]>;
};
