import { Navigate } from "react-router";
import { useDashboardHomePath } from "./useDashboardHomePath";

/** Resolves the dashboard root without changing explicit page links. */
export const DashboardIndexRedirect: React.FC = () => {
	return <Navigate to={useDashboardHomePath()} replace />;
};
