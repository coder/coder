import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
import { CreateGroupRequest } from "api/typesGenerated";

export const groups = (organizationId: string) => {
  return {
    queryKey: ["groups"],
    queryFn: () => API.getGroups(organizationId),
  };
};

export const createGroup = (queryClient: QueryClient) => {
  return {
    mutationFn: ({
      organizationId,
      ...request
    }: CreateGroupRequest & { organizationId: string }) =>
      API.createGroup(organizationId, request),
    onSuccess: async () => {
      await queryClient.invalidateQueries(["groups"]);
    },
  };
};
