import * as API from "api/api";
import { type WorkspacesResponse } from "api/typesGenerated";
import { type QueryOptions } from "react-query";

export function workspacesByQueryKey(query: string) {
  return ["workspaces", query] as const;
}

export function workspacesByQuery(query: string) {
  return {
    queryKey: workspacesByQueryKey(query),
    queryFn: () => API.getWorkspaces({ q: query }),
  } as const satisfies QueryOptions<WorkspacesResponse>;
}
