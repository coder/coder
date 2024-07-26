import type { Experiments } from "api/typesGenerated";

/**
 * @fileoverview TODO: centralize navigation code here! URL constants, URL formatting, all of it
 */

export const USERS_LINK = `/users?filter=${encodeURIComponent(
  "status:active",
)}`;

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
