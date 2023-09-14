import * as API from "api/api";
import { type Workspace } from "api/typesGenerated";
import { type QueryOptions } from "@tanstack/react-query";

export const workspaceByOwnerAndNameKey = (owner: string, name: string) => [
  "workspace",
  owner,
  name,
  "settings",
];

export const workspaceByOwnerAndName = (
  owner: string,
  name: string,
): QueryOptions<Workspace> => {
  return {
    queryKey: workspaceByOwnerAndNameKey(owner, name),
    queryFn: () => API.getWorkspaceByOwnerAndName(owner, name),
  };
};
