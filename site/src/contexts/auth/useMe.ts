import type { User } from "api/typesGenerated";
import { useAuth } from "./useAuth";

export const useMe = (): User => {
  const { user } = useAuth();

  if (!user) {
    throw new Error("User is not authenticated");
  }

  return user;
};
