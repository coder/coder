import { API } from "api/api";
import { tokens as tokensQuery } from "api/queries/tokens";
import type { TokensFilter } from "api/typesGenerated";
import {
	type QueryKey,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { tokens as tokensQuery } from "api/queries/tokens";

// Load all tokens
export const useTokensData = ({ include_all }: TokensFilter) => {
	const queryKey = ["tokens", include_all];
	const result = useQuery(tokensQuery({ include_all }));

	return {
		queryKey,
		...result,
	};
};

// Delete a token
export const useDeleteToken = (queryKey: QueryKey) => {
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: API.deleteToken,
		onSuccess: () => {
			// Invalidate and refetch
			void queryClient.invalidateQueries({
				queryKey,
			});
		},
	});
};
