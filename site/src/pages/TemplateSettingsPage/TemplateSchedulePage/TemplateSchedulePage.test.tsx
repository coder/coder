import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter";
import {
  MockEntitlementsWithScheduling,
  MockTemplate,
} from "testHelpers/entities";
import {
  renderWithTemplateSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import {
  getValidationSchema,
  type TemplateScheduleFormValues,
} from "./formHelpers";
import TemplateSchedulePage from "./TemplateSchedulePage";

const validFormValues: TemplateScheduleFormValues = {
  default_ttl_ms: 1,
  activity_bump_ms: 1,
  failure_ttl_ms: 7,
  time_til_dormant_ms: 180,
  time_til_dormant_autodelete_ms: 30,
  update_workspace_last_used_at: false,
  update_workspace_dormant_at: false,
  autostop_requirement_days_of_week: "off",
  autostop_requirement_weeks: 1,
  failure_cleanup_enabled: false,
  inactivity_cleanup_enabled: false,
  dormant_autodeletion_cleanup_enabled: false,
  require_active_version: false,
  autostart_requirement_days_of_week: [
    "monday",
    "tuesday",
    "wednesday",
    "thursday",
    "friday",
    "saturday",
    "sunday",
  ],
  disable_everyone_group_access: false,
};

const renderTemplateSchedulePage = async () => {
  renderWithTemplateSettingsLayout(<TemplateSchedulePage />, {
    route: `/templates/${MockTemplate.name}/settings/schedule`,
    path: `/templates/:template/settings/schedule`,
  });
  await waitForLoaderToBeRemoved();
};

// Extracts all properties from TemplateScheduleFormValues that have a key that
// ends in _ms, and makes those properties optional. Defined as mapped type to
// ensure this stays in sync as TemplateScheduleFormValues changes
type FillAndSubmitConfig = {
  [Key in keyof TemplateScheduleFormValues as Key extends `${string}_ms`
    ? Key
    : never]?: TemplateScheduleFormValues[Key] | undefined;
};

const fillAndSubmitForm = async ({
  default_ttl_ms,
  failure_ttl_ms,
  time_til_dormant_ms,
  time_til_dormant_autodelete_ms,
}: FillAndSubmitConfig) => {
  const user = userEvent.setup();

  if (default_ttl_ms) {
    const defaultTtlField = await screen.findByLabelText(
      "Default autostop (hours)",
    );
    await user.clear(defaultTtlField);
    await user.type(defaultTtlField, default_ttl_ms.toString());
  }

  if (failure_ttl_ms) {
    const failureTtlField = screen.getByRole("checkbox", {
      name: /Failure Cleanup/i,
    });
    await user.type(failureTtlField, failure_ttl_ms.toString());
  }

  if (time_til_dormant_ms) {
    const inactivityTtlField = screen.getByRole("checkbox", {
      name: /Dormancy Threshold/i,
    });
    await user.type(inactivityTtlField, time_til_dormant_ms.toString());
  }

  if (time_til_dormant_autodelete_ms) {
    const dormancyAutoDeletionField = screen.getByRole("checkbox", {
      name: /Dormancy Auto-Deletion/i,
    });
    await user.type(
      dormancyAutoDeletionField,
      time_til_dormant_autodelete_ms.toString(),
    );
  }

  const submitButton = screen.getByRole("button", {
    name: FooterFormLanguage.defaultSubmitLabel,
  });

  expect(submitButton).not.toBeDisabled();
  await user.click(submitButton);

  // User needs to confirm dormancy and auto-deletion fields.
  const confirmButton = await screen.findByTestId("confirm-button");
  await user.click(confirmButton);
};

// One problem with the waitFor function is that if no additional config options
// are passed in, it will hang indefinitely as it keeps retrying an assertion.
// Even if Jest runs out of time and kills the test, you won't get a good error
// message. Adding options to force test to give up before test timeout
function waitForWithCutoff(callback: () => void | Promise<void>) {
  return waitFor(callback, {
    // Defined to end 500ms before global cut-off time of 20s. Wanted to define
    // this in terms of an exported constant from jest.config, but since Jest
    // is CJS-based, that would've involved weird CJS-ESM interop issues
    timeout: 19_500,
  });
}

describe("TemplateSchedulePage", () => {
  beforeEach(() => {
    jest
      .spyOn(API, "getEntitlements")
      .mockResolvedValue(MockEntitlementsWithScheduling);
  });

  it("Calls the API when user fills in and submits a form", async () => {
    await renderTemplateSchedulePage();
    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    });

    await fillAndSubmitForm(validFormValues);
    await waitForWithCutoff(() =>
      expect(API.updateTemplateMeta).toBeCalledTimes(1),
    );
  });

  test("default is converted to and from hours", async () => {
    await renderTemplateSchedulePage();

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    });

    await fillAndSubmitForm(validFormValues);
    await waitForWithCutoff(() =>
      expect(API.updateTemplateMeta).toBeCalledTimes(1),
    );

    await waitForWithCutoff(() => {
      expect(API.updateTemplateMeta).toBeCalledWith(
        "test-template",
        expect.objectContaining({
          default_ttl_ms: (validFormValues.default_ttl_ms || 0) * 3600000,
        }),
      );
    });
  });

  test("failure, dormancy, and dormancy auto-deletion converted to and from days", async () => {
    await renderTemplateSchedulePage();

    jest.spyOn(API, "updateTemplateMeta").mockResolvedValueOnce({
      ...MockTemplate,
      ...validFormValues,
    });

    await fillAndSubmitForm(validFormValues);
    await waitForWithCutoff(() =>
      expect(API.updateTemplateMeta).toBeCalledTimes(1),
    );

    await waitForWithCutoff(() => {
      expect(API.updateTemplateMeta).toBeCalledWith(
        "test-template",
        expect.objectContaining({
          failure_ttl_ms: (validFormValues.failure_ttl_ms || 0) * 86400000,
          time_til_dormant_ms:
            (validFormValues.time_til_dormant_ms || 0) * 86400000,
          time_til_dormant_autodelete_ms:
            (validFormValues.time_til_dormant_autodelete_ms || 0) * 86400000,
        }),
      );
    });
  });

  it("allows a default ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 24 * 7,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows default ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 0,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows a default ttl of 30 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 24 * 30,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a default ttl of 30 days + 1 hour", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      default_ttl_ms: 24 * 30 + 1,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError(
      "Please enter a limit that is less than or equal to 720 hours (30 days).",
    );
  });

  it("allows a failure ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      failure_ttl_ms: 86400000 * 7,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows failure ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      failure_ttl_ms: 0,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a negative failure ttl", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      failure_ttl_ms: -1,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError(
      "Failure cleanup days must not be less than 0.",
    );
  });

  it("allows an inactivity ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_ms: 86400000 * 7,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows an inactivity ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_ms: 0,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a negative inactivity ttl", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_ms: -1,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError(
      "Dormancy threshold days must not be less than 0.",
    );
  });

  it("allows a dormancy ttl of 7 days", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_autodelete_ms: 86400000 * 7,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows a dormancy ttl of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_autodelete_ms: 0,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a negative inactivity ttl", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      time_til_dormant_autodelete_ms: -1,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError(
      "Dormancy auto-deletion days must not be less than 0.",
    );
  });

  it("allows an autostop requirement weeks of 1", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_days_of_week: "saturday",
      autostop_requirement_weeks: 1,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("allows a autostop requirement weeks of 16", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_weeks: 16,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).not.toThrowError();
  });

  it("disallows a autostop requirement weeks of 0", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_weeks: 0,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError();
  });

  it("disallows a autostop requirement weeks of 17", () => {
    const values: TemplateScheduleFormValues = {
      ...validFormValues,
      autostop_requirement_weeks: 17,
    };
    const validate = () => getValidationSchema().validateSync(values);
    expect(validate).toThrowError();
  });
});
