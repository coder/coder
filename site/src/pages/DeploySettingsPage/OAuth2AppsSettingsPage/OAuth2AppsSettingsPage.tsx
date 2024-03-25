import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { getApps } from "api/queries/oauth2";
import { pageTitle } from "utils/page";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const OAuth2AppsSettingsPage: FC = () => {
  const appsQuery = useQuery(getApps());

  return (
    <>
      <Helmet>
        <title>{pageTitle("OAuth2 Applications")}</title>
      </Helmet>
      <OAuth2AppsSettingsPageView
        apps={appsQuery.data}
        isLoading={appsQuery.isLoading}
        error={appsQuery.error}
      />
    </>
  );
};

export default OAuth2AppsSettingsPage;
