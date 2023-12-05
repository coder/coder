import * as API from "api/api";
import { HealthSettings } from "api/typesGenerated";
import { QueryClient, UseMutationOptions } from "react-query";

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

export const healthSettings = () => {
  return {
    queryKey: ["health", "settings"],
    queryFn: API.getHealthSettings,
  };
};

export const updateHealthSettings = (
  queryClient: QueryClient,
): UseMutationOptions<void, unknown, HealthSettings, unknown> => {
  return {
    mutationFn: API.updateHealthSettings,
    onSuccess: async (_, newSettings) => {
      await queryClient.invalidateQueries(["health"]);
      queryClient.setQueryData(["health", "settings"], newSettings);
    },
  };
};
