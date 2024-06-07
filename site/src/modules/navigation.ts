/**
 * @fileoverview TODO: centralize navigation code here! URL constants, URL formatting, all of it
 */

export const USERS_LINK = `/users?filter=${encodeURIComponent(
  "status:active",
)}`;
