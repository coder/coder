import type { StoryContext } from "@storybook/react";
import type { FC } from "react";
import { withDefaultFeatures } from "api/api";
import type { Entitlements } from "api/typesGenerated";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import { MockAppearanceConfig, MockEntitlements } from "./entities";

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
