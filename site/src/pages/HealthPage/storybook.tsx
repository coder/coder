import { useQueryClient } from "react-query";
import {
  reactRouterParameters,
  reactRouterOutlet,
  RouteDefinition,
} from "storybook-addon-react-router-v6";
import { MockHealth, MockHealthSettings } from "testHelpers/entities";
import { Meta } from "@storybook/react";
import { HEALTH_QUERY_KEY, HEALTH_QUERY_SETTINGS_KEY } from "api/queries/debug";

type MetaOptions = {
  element: RouteDefinition;
  path: string;
};

export const generateMeta = ({ element, path }: MetaOptions): Meta => {
  return {
    parameters: {
      layout: "fullscreen",
      reactRouter: reactRouterParameters({
        routing: reactRouterOutlet({ path: `/health/${path}` }, element),
      }),
    },
    decorators: [
      (Story) => {
        const queryClient = useQueryClient();
        queryClient.setQueryData(HEALTH_QUERY_KEY, MockHealth);
        queryClient.setQueryData(HEALTH_QUERY_SETTINGS_KEY, MockHealthSettings);
        return <Story />;
      },
    ],
  };
};
