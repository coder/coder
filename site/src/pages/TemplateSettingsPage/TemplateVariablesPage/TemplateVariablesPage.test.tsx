import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  renderWithTemplateSettingsLayout,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import * as API from "api/api";
import i18next from "i18next";
import TemplateVariablesPage from "./TemplateVariablesPage";
import { Language as FooterFormLanguage } from "components/FormFooter/FormFooter";
import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersion2,
  MockTemplateVersionVariable5,
} from "testHelpers/entities";

const { t } = i18next;

const validFormValues = {
  first_variable: "Hello world",
  second_variable: "123",
};

const pageTitleText = t("title", { ns: "templateVariablesPage" });

const validationRequiredField = t("validationRequiredVariable", {
  ns: "templateVariablesPage",
});

const renderTemplateVariablesPage = async () => {
  renderWithTemplateSettingsLayout(<TemplateVariablesPage />, {
    route: `/templates/${MockTemplate.name}/variables`,
    path: `/templates/:template/variables`,
    extraRoutes: [{ path: `/templates/${MockTemplate.name}`, element: <></> }],
  });
  await waitForLoaderToBeRemoved();
};

describe("TemplateVariablesPage", () => {
  it("renders with variables", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValueOnce(MockTemplate);
    jest
      .spyOn(API, "getTemplateVersion")
      .mockResolvedValueOnce(MockTemplateVersion);
    jest
      .spyOn(API, "getTemplateVersionVariables")
      .mockResolvedValueOnce([
        MockTemplateVersionVariable1,
        MockTemplateVersionVariable2,
      ]);

    await renderTemplateVariablesPage();

    const element = await screen.findByText(pageTitleText);
    expect(element).toBeDefined();

    const firstVariable = await screen.findByLabelText(
      MockTemplateVersionVariable1.name,
    );
    expect(firstVariable).toBeDefined();

    const secondVariable = await screen.findByLabelText(
      MockTemplateVersionVariable2.name,
    );
    expect(secondVariable).toBeDefined();
  });

  it("user submits the form successfully", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValueOnce(MockTemplate);
    jest
      .spyOn(API, "getTemplateVersion")
      .mockResolvedValueOnce(MockTemplateVersion);
    jest
      .spyOn(API, "getTemplateVersionVariables")
      .mockResolvedValueOnce([
        MockTemplateVersionVariable1,
        MockTemplateVersionVariable2,
      ]);
    jest
      .spyOn(API, "createTemplateVersion")
      .mockResolvedValueOnce(MockTemplateVersion2);
    jest.spyOn(API, "updateActiveTemplateVersion").mockResolvedValueOnce({
      message: "done",
    });

    await renderTemplateVariablesPage();

    const element = await screen.findByText(pageTitleText);
    expect(element).toBeDefined();

    const firstVariable = await screen.findByLabelText(
      MockTemplateVersionVariable1.name,
    );
    expect(firstVariable).toBeDefined();

    const secondVariable = await screen.findByLabelText(
      MockTemplateVersionVariable2.name,
    );
    expect(secondVariable).toBeDefined();

    // Fill the form
    const firstVariableField = await screen.findByLabelText(
      MockTemplateVersionVariable1.name,
    );
    await userEvent.clear(firstVariableField);
    await userEvent.type(firstVariableField, validFormValues.first_variable);

    const secondVariableField = await screen.findByLabelText(
      MockTemplateVersionVariable2.name,
    );
    await userEvent.clear(secondVariableField);
    await userEvent.type(secondVariableField, validFormValues.second_variable);

    // Submit the form
    const submitButton = await screen.findByText(
      FooterFormLanguage.defaultSubmitLabel,
    );
    await userEvent.click(submitButton);

    // Wait for the success message
    await screen.findByText("Template updated successfully");
  });

  it("user forgets to fill the required field", async () => {
    jest.spyOn(API, "getTemplateByName").mockResolvedValueOnce(MockTemplate);
    jest
      .spyOn(API, "getTemplateVersion")
      .mockResolvedValueOnce(MockTemplateVersion);
    jest
      .spyOn(API, "getTemplateVersionVariables")
      .mockResolvedValueOnce([
        MockTemplateVersionVariable1,
        MockTemplateVersionVariable5,
      ]);
    jest
      .spyOn(API, "createTemplateVersion")
      .mockResolvedValueOnce(MockTemplateVersion2);
    jest.spyOn(API, "updateActiveTemplateVersion").mockResolvedValueOnce({
      message: "done",
    });

    await renderTemplateVariablesPage();

    const element = await screen.findByText(pageTitleText);
    expect(element).toBeDefined();

    const firstVariable = await screen.findByLabelText(
      MockTemplateVersionVariable1.name,
    );
    expect(firstVariable).toBeDefined();

    const fifthVariable = await screen.findByLabelText(
      MockTemplateVersionVariable5.name,
    );
    expect(fifthVariable).toBeDefined();

    // Submit the form
    const submitButton = await screen.findByText(
      FooterFormLanguage.defaultSubmitLabel,
    );
    await userEvent.click(submitButton);

    // Check validation error
    const validationError = await screen.findByText(validationRequiredField);
    expect(validationError).toBeDefined();
  });
});
