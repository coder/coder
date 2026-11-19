import { useQuery } from "react-query";
import { preferenceSettings } from "#/api/queries/users";
import { disabledRefetchOptions } from "#/api/queries/util";
import { useAuthenticated } from "#/hooks/useAuthenticated";

/** Resolves dashboard navigation from cached preferences or an initial read. */
export const useDashboardHomePath = () => {
	const { permissions } = useAuthenticated();
	const query = useQuery({
		...preferenceSettings(),
		...disabledRefetchOptions,
		enabled: permissions.createChat,
		retry: false,
		retryOnMount: false,
	});

	return {
		homePath:
			query.data?.default_landing_page === "agents" && permissions.createChat
				? "/agents"
				: "/workspaces",
		isLoading: permissions.createChat && query.isLoading,
	};
};
