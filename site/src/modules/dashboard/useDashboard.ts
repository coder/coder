import { useContext } from "react";
import { DashboardContext, type DashboardValue } from "./DashboardProvider";

export const useDashboard = (): DashboardValue => {
	const context = useContext(DashboardContext);

	if (!context) {
		throw new Error(
			"useDashboard only can be used inside of DashboardProvider",
		);
	}

	return context;
};

export const getDefaultOrganizationName = (
	organizations: DashboardValue["organizations"],
): string => organizations.find((org) => org.is_default)?.name ?? "";
