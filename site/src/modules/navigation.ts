/**
 * @fileoverview TODO: centralize navigation code here! URL constants, URL formatting, all of it
 */

export function withFilter(path: string, filter: string) {
  return path + (filter ? `?filter=${encodeURIComponent(filter)}` : "");
}

export const AUDIT_LINK = "/audit";

export const USERS_LINK = withFilter("/users", "status:active");
