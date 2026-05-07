/**
 * @fileoverview TODO: centralize navigation code here! URL constants, URL formatting, all of it
 */

import { useCallback } from "react";
import type { DashboardValue } from "./dashboard/DashboardProvider";
import { useDashboard } from "./dashboard/useDashboard";

type LinkThunk = (state: DashboardValue) => string;

export function useLinks() {
	const dashboard = useDashboard();
	const get = useCallback(
		(thunk: LinkThunk): string => thunk(dashboard),
		[dashboard],
	);
	return get;
}

function withFilter(path: string, filter: string) {
	return path + (filter ? `?filter=${encodeURIComponent(filter)}` : "");
}

export const linkToAuditing = "/audit";

const _linkToUsers = withFilter("/deployment/users", "status:active");

export const linkToTemplate =
	(organizationName: string, templateName: string): LinkThunk =>
	(dashboard) =>
		dashboard.showOrganizations
			? `/templates/${organizationName}/${templateName}`
			: `/templates/${templateName}`;
