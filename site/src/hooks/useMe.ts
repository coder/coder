import type { User } from "api/typesGenerated";
import { useAuth } from "contexts/AuthProvider/AuthProvider";

export const useMe = (): User => {
  const { user } = useAuth();

  if (!user) {
    throw new Error("User is not authenticated");
  }

  return user;
};
