import * as API from "api/api";
import { UpdateUserPasswordRequest } from "api/typesGenerated";

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
