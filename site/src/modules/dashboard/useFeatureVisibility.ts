import { FeatureName } from "api/typesGenerated";
import { useDashboard } from "./useDashboard";
import { selectFeatureVisibility } from "utils/entitlements";

export const useFeatureVisibility = (): Record<FeatureName, boolean> => {
  const { entitlements } = useDashboard();
  return selectFeatureVisibility(entitlements);
};
