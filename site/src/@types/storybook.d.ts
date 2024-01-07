import * as _storybook_types from "@storybook/react";
import { Experiments, FeatureName } from "api/typesGenerated";
import { QueryKey } from "react-query";

declare module "@storybook/react" {
  interface Parameters {
    features?: FeatureName[];
    experiments?: Experiments;
    queries?: { key: QueryKey; data: unknown }[];
  }
}
