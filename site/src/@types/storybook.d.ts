import * as _storybook_types from "@storybook/react";
import type { QueryKey } from "react-query";
import type { Experiments, FeatureName } from "api/typesGenerated";

declare module "@storybook/react" {
  interface Parameters {
    features?: FeatureName[];
    experiments?: Experiments;
    queries?: { key: QueryKey; data: unknown }[];
    webSocket?: {
      messages: string[];
    };
  }
}
