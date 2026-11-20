import { Navigate } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { useDashboardHomePath } from "./useDashboardHomePath";

/** Resolves the dashboard root without changing explicit page links. */
export const DashboardIndexRedirect: React.FC = () => {
	const { homePath, isLoading } = useDashboardHomePath();
	if (isLoading) {
		return <Loader label="Loading default landing page" />;
	}
	return <Navigate to={homePath} replace />;
};
