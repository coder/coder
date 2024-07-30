/**
 * @fileoverview TODO: centralize navigation code here! URL constants, URL formatting, all of it
 */
import type { Experiments } from "api/typesGenerated";

export function withFilter(path: string, filter: string) {
  return path + (filter ? `?filter=${encodeURIComponent(filter)}` : "");
}

export const AUDIT_LINK = "/audit";

export const USERS_LINK = withFilter("/users", "status:active");

export const TEMPLATES_ROUTE = (
  organizationId: string,
  templateName: string,
  routeSuffix: string = "",
  orgsEnabled: boolean = false,
  experiments: Experiments = [],
) => {
  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  if (multiOrgExperimentEnabled && orgsEnabled) {
    return `/templates/${organizationId}/${templateName}${routeSuffix}`;
  }

  return `/templates/${templateName}${routeSuffix}`;
};
