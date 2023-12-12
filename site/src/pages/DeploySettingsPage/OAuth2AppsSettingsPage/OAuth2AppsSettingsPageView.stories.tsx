import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";
import { MockOAuth2Apps } from "testHelpers/entities";

export default {
  title: "pages/DeploySettingsPage/OAuth2AppsSettingsPageView",
  component: OAuth2AppsSettingsPageView,
};

export const Loading = {
  args: {
    isLoading: true,
  },
};

export const Unentitled = {
  args: {
    isLoading: false,
    apps: MockOAuth2Apps,
  },
};

export const Entitled = {
  args: {
    isLoading: false,
    apps: MockOAuth2Apps,
    isEntitled: true,
  },
};

export const Empty = {
  args: {
    isLoading: false,
    apps: null,
  },
};
