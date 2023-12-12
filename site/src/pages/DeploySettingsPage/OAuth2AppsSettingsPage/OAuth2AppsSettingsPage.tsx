import { useQuery } from "react-query";
import { oauth2Apps } from "api/queries/oauth2";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const OAuth2AppsSettingsPage: FC = () => {
  const { entitlements } = useDashboard();
  const oauth2AppsQuery = useQuery(oauth2Apps());

  return (
    <>
      <Helmet>
        <title>{pageTitle("OAuth2 Applications")}</title>
      </Helmet>
      <OAuth2AppsSettingsPageView
        apps={oauth2AppsQuery.data}
        isLoading={oauth2AppsQuery.isLoading}
        isEntitled={
          entitlements.features.oauth2_provider.entitlement !== "not_entitled"
        }
      />
    </>
  );
};

export default OAuth2AppsSettingsPage;
