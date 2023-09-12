import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";

const ENTITLEMENTS_QUERY_KEY = ["entitlements"];

export const entitlements = () => {
  return {
    queryKey: ENTITLEMENTS_QUERY_KEY,
    queryFn: fetchEntitlements,
  };
};

export const refreshEntitlements = (queryClient: QueryClient) => {
  return {
    mutationFn: API.refreshEntitlements,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ENTITLEMENTS_QUERY_KEY,
      });
    },
  };
};

const fetchEntitlements = () => {
  // Entitlements is injected by the Coder server into the HTML document.
  const entitlements = document.querySelector("meta[property=entitlements]");
  if (entitlements) {
    const rawContent = entitlements.getAttribute("content");
    try {
      return JSON.parse(rawContent as string);
    } catch (e) {
      console.warn("Failed to parse entitlements from document", e);
    }
  }

  return API.getEntitlements();
};
