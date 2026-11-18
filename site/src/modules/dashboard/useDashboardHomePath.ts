import { useQuery } from "react-query";
import { preferenceSettings } from "#/api/queries/users";
import { useAuthenticated } from "#/hooks/useAuthenticated";

/** Reads the home destination without starting or waiting for a preference request. */
export const useDashboardHomePath = () => {
	const { permissions } = useAuthenticated();
	const query = useQuery({
		...preferenceSettings(),
		enabled: false,
	});

	return query.data?.default_landing_page === "agents" && permissions.createChat
		? "/agents"
		: "/workspaces";
};
