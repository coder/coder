import type { FeatureName } from "api/typesGenerated";
import { selectFeatureVisibility } from "./entitlements";
import { useDashboard } from "./useDashboard";

export const useFeatureVisibility = (): Record<FeatureName, boolean> => {
  const { entitlements } = useDashboard();
  return selectFeatureVisibility(entitlements);
};
