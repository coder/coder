import { API } from "api/api";
import type { TokensFilter } from "api/typesGenerated";
import {
	type QueryKey,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";

// Load all tokens
export const useTokensData = ({
	include_all,
	include_expired,
}: TokensFilter) => {
	const queryKey = ["tokens", include_all, include_expired];
	const result = useQuery({
		queryKey,
		queryFn: () => API.getTokens({ include_all, include_expired }),
	});

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
