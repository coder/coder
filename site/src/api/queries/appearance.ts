import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
import { AppearanceConfig } from "api/typesGenerated";

export const appearance = () => {
  return {
    queryKey: ["appearance"],
    queryFn: fetchAppearance,
  };
};

export const updateAppearance = (queryClient: QueryClient) => {
  return {
    mutationFn: API.updateAppearance,
    onSuccess: (newConfig: AppearanceConfig) => {
      queryClient.setQueryData(["appearance"], newConfig);
    },
  };
};

const fetchAppearance = () => {
  const appearance = document.querySelector("meta[property=appearance]");
  if (appearance) {
    const rawContent = appearance.getAttribute("content");
    try {
      return JSON.parse(rawContent as string);
    } catch (ex) {
      // Ignore this and fetch as normal!
    }
  }

  return API.getAppearance();
};
