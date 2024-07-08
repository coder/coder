import type { FC } from "react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { LicenseBannerView } from "./LicenseBannerView";

export const LicenseBanner: FC = () => {
  const { entitlements } = useDashboard();
  const { errors, warnings } = entitlements;

  if (errors.length === 0 && warnings.length === 0) {
    return null;
  }

  return <LicenseBannerView errors={errors} warnings={warnings} />;
};
