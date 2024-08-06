import type { StoryContext } from "@storybook/react";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { withDefaultFeatures } from "api/api";
import { getAuthorizationKey } from "api/queries/authCheck";
import { hasFirstUserKey, meKey } from "api/queries/users";
import type { Entitlements } from "api/typesGenerated";
import { GlobalSnackbar } from "components/GlobalSnackbar/GlobalSnackbar";
import { AuthProvider } from "contexts/auth/AuthProvider";
import { permissionsToCheck } from "contexts/auth/permissions";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import { DeploySettingsContext } from "pages/DeploySettingsPage/DeploySettingsLayout";
import {
  MockAppearanceConfig,
  MockDefaultOrganization,
  MockEntitlements,
} from "./entities";

export const withDashboardProvider = (
  Story: FC,
  { parameters }: StoryContext,
) => {
  const { features = [], experiments = [] } = parameters;

  const entitlements: Entitlements = {
    ...MockEntitlements,
    features: withDefaultFeatures(
      Object.fromEntries(
        features.map((feature) => [
          feature,
          { enabled: true, entitlement: "entitled" },
        ]),
      ),
    ),
  };

  return (
    <DashboardContext.Provider
      value={{
        entitlements,
        experiments,
        appearance: MockAppearanceConfig,
        organizations: [MockDefaultOrganization],
      }}
    >
      <Story />
    </DashboardContext.Provider>
  );
};

type MessageEvent = Record<"data", string>;
type CallbackFn = (ev?: MessageEvent) => void;

export const withWebSocket = (Story: FC, { parameters }: StoryContext) => {
  const events = parameters.webSocket;

  if (!events) {
    console.warn("You forgot to add `parameters.webSocket` to your story");
    return <Story />;
  }

  const listeners = new Map<string, CallbackFn>();
  let callEventsDelay: number;

  // @ts-expect-error -- TS doesn't know about the global WebSocket
  window.WebSocket = function () {
    return {
      addEventListener: (type: string, callback: CallbackFn) => {
        listeners.set(type, callback);

        // Runs when the last event listener is added
        clearTimeout(callEventsDelay);
        callEventsDelay = window.setTimeout(() => {
          for (const entry of events) {
            const callback = listeners.get(entry.event);

            if (callback) {
              entry.event === "message"
                ? callback({ data: entry.data })
                : callback();
            }
          }
        }, 0);
      },
      close: () => {},
    };
  };

  return <Story />;
};

export const withDesktopViewport = (Story: FC) => (
  <div style={{ width: 1200, height: 800 }}>
    <Story />
  </div>
);

export const withAuthProvider = (Story: FC, { parameters }: StoryContext) => {
  if (!parameters.user) {
    console.warn("You forgot to add `parameters.user` to your story");
  }
  // eslint-disable-next-line react-hooks/rules-of-hooks -- decorators are components
  const queryClient = useQueryClient();
  queryClient.setQueryData(meKey, parameters.user);
  queryClient.setQueryData(hasFirstUserKey, true);
  queryClient.setQueryData(
    getAuthorizationKey({ checks: permissionsToCheck }),
    parameters.permissions ?? {},
  );

  return (
    <AuthProvider>
      <Story />
    </AuthProvider>
  );
};

export const withGlobalSnackbar = (Story: FC) => (
  <>
    <Story />
    <GlobalSnackbar />
  </>
);

export const withDeploySettings = (Story: FC, { parameters }: StoryContext) => {
  return (
    <DeploySettingsContext.Provider
      value={{
        deploymentValues: {
          config: parameters.deploymentValues ?? {},
          options: parameters.deploymentOptions ?? [],
        },
      }}
    >
      <Story />
    </DeploySettingsContext.Provider>
  );
};
