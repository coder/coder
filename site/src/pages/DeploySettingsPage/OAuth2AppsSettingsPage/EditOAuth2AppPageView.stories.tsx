import { EditOAuth2AppPageView } from "./EditOAuth2AppPageView";
import {
  MockOAuth2Apps,
  MockOAuth2AppSecrets,
  mockApiError,
} from "testHelpers/entities";

export default {
  title: "pages/DeploySettingsPage/EditOAuth2AppPageView",
  component: EditOAuth2AppPageView,
};

export const LoadingApp = {
  args: {
    isLoadingApp: true,
  },
};

export const LoadingSecrets = {
  args: {
    app: MockOAuth2Apps[0],
    isLoadingSecrets: true,
  },
};

export const Error = {
  args: {
    app: MockOAuth2Apps[0],
    secrets: MockOAuth2AppSecrets,
    error: mockApiError({
      message: "Validation failed",
      validations: [
        {
          field: "name",
          detail: "name error",
        },
        {
          field: "callback_url",
          detail: "url error",
        },
        {
          field: "icon",
          detail: "icon error",
        },
      ],
    }),
  },
};

export const Default = {
  args: {
    app: MockOAuth2Apps[0],
    secrets: MockOAuth2AppSecrets,
  },
};
