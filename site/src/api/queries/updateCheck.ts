import { client } from "api/api";

export const updateCheck = () => {
  return {
    queryKey: ["updateCheck"],
    queryFn: () => client.api.getUpdateCheck(),
  };
};
