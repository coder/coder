import { client } from "api/api";

export const roles = () => {
  return {
    queryKey: ["roles"],
    queryFn: client.api.getRoles,
  };
};
