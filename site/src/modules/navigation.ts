/**
 * @fileoverview TODO: centralize navigation code here! URL constants, URL formatting, all of it
 */

import type { DashboardValue } from "./dashboard/DashboardProvider";
import { selectFeatureVisibility } from "./dashboard/entitlements";
import { useDashboard } from "./dashboard/useDashboard";

type LinkThunk = (state: DashboardValue) => string;

export function useLinks() {
  const dashboard = useDashboard();
  return (thunk: LinkThunk): string => thunk(dashboard);
}

export function withFilter(path: string, filter: string) {
  return path + (filter ? `?filter=${encodeURIComponent(filter)}` : "");
}

export const linkToAuditing = "/audit";

export const linkToUsers = withFilter("/users", "status:active");

export const linkToTemplate =
  (organizationName: string, templateName: string): LinkThunk =>
  (dashboard) =>
    dashboard.experiments.includes("multi-organization") &&
    selectFeatureVisibility(dashboard.entitlements).multiple_organizations
      ? `/templates/${organizationName}/${templateName}`
      : `/templates/${templateName}`;
