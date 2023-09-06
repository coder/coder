import { FeatureName } from "api/typesGenerated";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors";

export const useFeatureVisibility = (): Record<FeatureName, boolean> => {
  const { entitlements } = useDashboard();
  return selectFeatureVisibility(entitlements);
};
