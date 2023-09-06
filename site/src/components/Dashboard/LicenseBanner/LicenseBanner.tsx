import { useDashboard } from "components/Dashboard/DashboardProvider"
import { LicenseBannerView } from "./LicenseBannerView"

export const LicenseBanner: React.FC = () => {
  const { entitlements } = useDashboard()
  const { errors, warnings } = entitlements

  if (errors.length > 0 || warnings.length > 0) {
    return <LicenseBannerView errors={errors} warnings={warnings} />
  } else {
    return null
  }
}
