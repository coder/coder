import type { FC } from "react";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { LicenseBannerView } from "./LicenseBannerView";

export const LicenseBanner: FC = () => {
  const { permissions } = useAuthenticated();
  const { entitlements } = useDashboard();
  const { errors, deployment_warnings, operator_warnings } = entitlements;

  if (
    errors.length === 0 &&
    deployment_warnings.length === 0 &&
    operator_warnings.length === 0
  ) {
    return null;
  }

  return (
    <LicenseBannerView
      errors={errors}
      // Only display deployment warnings if the user has permission to view deployment values.
      // Otherwise, the user likely cannot take action anyways.
      operator_warnings={
        permissions.viewDeploymentValues ? operator_warnings : []
      }
      deployment_warnings={deployment_warnings}
    />
  );
};
