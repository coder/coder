import { useQuery } from "react-query";
import { Navigate } from "react-router";
import { preferenceSettings } from "#/api/queries/users";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";

/** Resolves the dashboard root without changing explicit page links. */
export const DashboardIndexRedirect: React.FC = () => {
	const { permissions } = useAuthenticated();
	const query = useQuery({
		...preferenceSettings(),
		retry: false,
	});

	if (query.isLoading) {
		return <Loader />;
	}

	return (
		<Navigate
			to={
				query.data?.default_landing_page === "agents" && permissions.createChat
					? "/agents"
					: "/workspaces"
			}
			replace
		/>
	);
};
