import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";
import { mockApiError } from "testHelpers/entities";

export default {
  title: "pages/DeploySettingsPage/CreateOAuth2AppPageView",
  component: CreateOAuth2AppPageView,
};

export const Updating = {
  args: {
    isUpdating: true,
  },
};

export const Error = {
  args: {
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
    // Nothing.
  },
};
