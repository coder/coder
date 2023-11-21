import * as API from "api/api";
import { QueryClient } from "react-query";

export const health = () => ({
  queryKey: ["health"],
  queryFn: async () => API.getHealth(),
});

export const refreshHealth = (queryClient: QueryClient) => {
  return {
    mutationFn: async () => {
      await queryClient.cancelQueries(["health"]);
      const newHealthData = await API.getHealth(true);
      queryClient.setQueryData(["health"], newHealthData);
    },
  };
};
