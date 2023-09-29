import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
import { UpdateUserPasswordRequest, UsersRequest } from "api/typesGenerated";

export const users = (req: UsersRequest) => {
  return {
    queryKey: ["users", req],
    queryFn: () => API.getUsers(req),
  };
};

export const updatePassword = () => {
  return {
    mutationFn: ({
      userId,
      ...request
    }: UpdateUserPasswordRequest & { userId: string }) =>
      API.updateUserPassword(userId, request),
  };
};

export const createUser = () => {
  return {
    mutationFn: API.createUser,
  };
};

export const createFirstUser = () => {
  return {
    mutationFn: API.createFirstUser,
  };
};

export const suspendUser = (queryClient: QueryClient) => {
  return {
    mutationFn: API.suspendUser,
    onSuccess: async () => {
      await queryClient.invalidateQueries(["users"]);
    },
  };
};

export const activateUser = (queryClient: QueryClient) => {
  return {
    mutationFn: API.activateUser,
    onSuccess: async () => {
      await queryClient.invalidateQueries(["users"]);
    },
  };
};
