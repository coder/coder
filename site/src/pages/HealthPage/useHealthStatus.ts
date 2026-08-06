import { useQuery } from "react-query";
import { health } from "#/api/queries/debug";

/**
 * Shared healthcheck query for the Health sidebar and pages. React Query
 * dedupes on the same key so both surfaces stay in sync.
 */
export const useHealthStatus = () => {
	return useQuery({
		...health(),
		refetchInterval: 30_000,
	});
};
