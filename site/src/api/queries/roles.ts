import { API } from "api/api";

export const roles = () => {
  return {
    queryKey: ["roles"],
    queryFn: API.getRoles,
  };
};
