import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import * as API from "api/api";
import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter";
import { MockEntitlements, MockTemplate } from "testHelpers/entities";
import {
  renderWithTemplateSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { getValidationSchema } from "./TemplateSettingsForm";
import { TemplateSettingsPage } from "./TemplateSettingsPage";

type FormValues = Required<
  Omit<
    UpdateTemplateMeta,
    "default_ttl_ms" | "activity_bump_ms" | "deprecation_message"
  >
>;

const validFormValues: FormValues = {
  name: "Name",
  display_name: "A display name",
  description: "A description",
  icon: "vscode.png",
  allow_user_cancel_workspace_jobs: false,
  allow_user_autostart: false,
  allow_user_autostop: false,
  autostop_requirement: {
    days_of_week: [],
    weeks: 1,
  },
  autostart_requirement: {
    days_of_week: [
      "monday",
      "tuesday",
      "wednesday",
      "thursday",
      "friday",
      "saturday",
      "sunday",
    ],
  },
  failure_ttl_ms: 0,
  time_til_dormant_ms: 0,
  time_til_dormant_autodelete_ms: 0,
  update_workspace_last_used_at: false,
  update_workspace_dormant_at: false,
  require_active_version: false,
  disable_everyone_group_access: false,
  max_port_share_level: "owner",
};

const renderTemplateSettingsPage = async () => {
  renderWithTemplateSettingsLayout(<TemplateSettingsPage />, {
    route: `/templates/${MockTemplate.name}/settings`,
    path: `/templates/:template/settings`,
    extraRoutes: [
      { path: "/templates/:template", element: <div>Template</div> },
    ],
  });
  await waitForLoaderToBeRemoved();
};

const fillAndSubmitForm = async ({
  name,
  display_name,
  description,
  icon,
  allow_user_cancel_workspace_jobs,
}: FormValues) => {
  const nameField = await screen.findByLabelText("Name");
  await userEvent.clear(nameField);
  await userEvent.type(nameField, name);

  const displayNameField = await screen.findByLabelText("Display name");
  await userEvent.clear(displayNameField);
  await userEvent.type(displayNameField, display_name);

  const descriptionField = await screen.findByLabelText("Description");
  await userEvent.clear(descriptionField);
  await userEvent.type(descriptionField, description);

  const iconField = await screen.findByLabelText("Icon");
  await userEvent.clear(iconField);
  await userEvent.type(iconField, icon);

  const allowCancelJobsField = screen.getByRole("checkbox", {
    name: /allow users to cancel in-progress workspace jobs/i,
  });
  // checkbox is checked by default, so it must be clicked to get unchecked
  if (!allow_user_cancel_workspace_jobs) {
    await userEvent.click(allowCancelJobsField);
  }

  const submitButton = await screen.findByText(
    FooterFormLanguage.defaultSubmitLabel,
  );
  await userEvent.click(submitButton);
};

describe("TemplateSettingsPage", () => {
  it("succeeds", async () => {
    await renderTemplateSettingsPage();
    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    });
    await fillAndSubmitForm(validFormValues);
    await waitFor(() => expect(API.updateTemplateMeta).toBeCalledTimes(1));
  });

  it("allows a description of 128 chars", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      description:
        "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus molestie, port",
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a description of 128 + 1 chars", () => {
    const values: UpdateTemplateMeta = {
      ...validFormValues,
      description:
        "Nam quis nulla. Integer malesuada. In in enim a arcu imperdiet malesuada. Sed vel lectus. Donec odio urna, tempus molestie, port a",
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError();
  });

  describe("Deprecate template", () => {
    it("deprecates a template when has access control", async () => {
      server.use(
        http.get("/api/v2/entitlements", () => {
          return HttpResponse.json({
            ...MockEntitlements,
            features: API.withDefaultFeatures({
              access_control: { enabled: true, entitlement: "entitled" },
            }),
          });
        }),
      );
      const updateTemplateMetaSpy = jest.spyOn(API, "updateTemplateMeta");
      const deprecationMessage = "This template is deprecated";

      await renderTemplateSettingsPage();
      await deprecateTemplate(MockTemplate, deprecationMessage);

      const [templateId, data] = updateTemplateMetaSpy.mock.calls[0];

      expect(templateId).toEqual(MockTemplate.id);
      expect(data).toEqual(
        expect.objectContaining({ deprecation_message: deprecationMessage }),
      );
    });

    it("does not deprecate a template when does not have access control", async () => {
      server.use(
        http.get("/api/v2/entitlements", () => {
          return HttpResponse.json({
            ...MockEntitlements,
            features: API.withDefaultFeatures({
              access_control: { enabled: false, entitlement: "not_entitled" },
            }),
          });
        }),
      );
      const updateTemplateMetaSpy = jest.spyOn(API, "updateTemplateMeta");

      await renderTemplateSettingsPage();
      await deprecateTemplate(
        MockTemplate,
        "This template should not be able to deprecate",
      );

      const [templateId, data] = updateTemplateMetaSpy.mock.calls[0];

      expect(templateId).toEqual(MockTemplate.id);
      expect(data).toEqual(
        expect.objectContaining({ deprecation_message: "" }),
      );
    });
  });
});

async function deprecateTemplate(template: Template, message: string) {
  const deprecationField = screen.getByLabelText("Deprecation Message");
  await userEvent.type(deprecationField, message);

  const submitButton = await screen.findByText(
    FooterFormLanguage.defaultSubmitLabel,
  );
  await userEvent.click(submitButton);
}
