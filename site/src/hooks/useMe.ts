import { User } from "api/typesGenerated";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { isAuthenticated } from "xServices/auth/authXService";

export const useMe = (): User => {
  const [authState] = useAuth();
  const { data } = authState.context;

  if (isAuthenticated(data)) {
    return data.user;
  }

  throw new Error("User is not authenticated");
};
