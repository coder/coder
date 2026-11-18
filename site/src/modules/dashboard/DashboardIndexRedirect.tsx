import { Navigate } from "react-router";
import { useDashboardHomePath } from "./useDashboardHomePath";

/** Resolves the dashboard root without changing explicit page links. */
export const DashboardIndexRedirect: React.FC = () => {
	const homePath = useDashboardHomePath();
	return <Navigate to={homePath} replace />;
};
