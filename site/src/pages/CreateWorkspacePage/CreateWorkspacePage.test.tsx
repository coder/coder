import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as API from "api/api";
import i18next from "i18next";
import {
  MockTemplate,
  MockUser,
  MockWorkspace,
  MockWorkspaceQuota,
  MockWorkspaceRequest,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockTemplateVersionGitAuth,
  MockOrganization,
} from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import CreateWorkspacePage from "./CreateWorkspacePage";

const { t } = i18next;

const nameLabelText = t("nameLabel", { ns: "createWorkspacePage" });
const createWorkspaceText = t("createWorkspace", { ns: "createWorkspacePage" });
const validationNumberNotInRangeText = t("validationNumberNotInRange", {
  ns: "createWorkspacePage",
  min: "1",
  max: "3",
});
const validationPatternNotMatched = t("validationPatternNotMatched", {
  ns: "createWorkspacePage",
  error: MockTemplateVersionParameter3.validation_error,
  pattern: "^[a-z]{3}$",
});

const renderCreateWorkspacePage = () => {
  return renderWithAuth(<CreateWorkspacePage />, {
    route: "/templates/" + MockTemplate.name + "/workspace",
    path: "/templates/:template/workspace",
  });
};

Object.defineProperty(window, "BroadcastChannel", {
  value: class {
    addEventListener() {
      // noop
    }
    close() {
      // noop
    }
  },
});

describe("CreateWorkspacePage", () => {
  it("succeeds with default owner", async () => {
    jest
      .spyOn(API, "getUsers")
      .mockResolvedValueOnce({ users: [MockUser], count: 1 });
    jest
      .spyOn(API, "getWorkspaceQuota")
      .mockResolvedValueOnce(MockWorkspaceQuota);
    jest.spyOn(API, "createWorkspace").mockResolvedValueOnce(MockWorkspace);
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([MockTemplateVersionParameter1]);

    renderCreateWorkspacePage();

    const nameField = await screen.findByLabelText(nameLabelText);

    // have to use fireEvent b/c userEvent isn't cleaning up properly between tests
    fireEvent.change(nameField, {
      target: { value: "test" },
    });

    const submitButton = screen.getByText(createWorkspaceText);
    await userEvent.click(submitButton);

    await waitFor(() =>
      expect(API.createWorkspace).toBeCalledWith(
        MockUser.organization_ids[0],
        MockUser.id,
        {
          ...MockWorkspaceRequest,
        },
      ),
    );
  });

  it("uses default rich param values passed from the URL", async () => {
    const param = "first_parameter";
    const paramValue = "It works!";
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([MockTemplateVersionParameter1]);

    renderWithAuth(<CreateWorkspacePage />, {
      route:
        "/templates/" +
        MockTemplate.name +
        `/workspace?param.${param}=${paramValue}`,
      path: "/templates/:template/workspace",
    });

    await screen.findByDisplayValue(paramValue);
  });

  it("rich parameter: number validation fails", async () => {
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter2,
      ]);

    renderCreateWorkspacePage();
    await waitForLoaderToBeRemoved();

    const element = await screen.findByText("Create Workspace");
    expect(element).toBeDefined();
    const secondParameter = await screen.findByText(
      MockTemplateVersionParameter2.description,
    );
    expect(secondParameter).toBeDefined();

    const secondParameterField = await screen.findByLabelText(
      MockTemplateVersionParameter2.name,
      { exact: false },
    );
    expect(secondParameterField).toBeDefined();

    fireEvent.change(secondParameterField, {
      target: { value: "4" },
    });
    fireEvent.submit(secondParameter);

    const validationError = await screen.findByText(
      validationNumberNotInRangeText,
    );
    expect(validationError).toBeDefined();
  });

  it("rich parameter: string validation fails", async () => {
    jest
      .spyOn(API, "getTemplateVersionRichParameters")
      .mockResolvedValueOnce([
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter3,
      ]);

    renderCreateWorkspacePage();
    await waitForLoaderToBeRemoved();

    const element = await screen.findByText(createWorkspaceText);
    expect(element).toBeDefined();
    const thirdParameter = await screen.findByText(
      MockTemplateVersionParameter3.description,
    );
    expect(thirdParameter).toBeDefined();

    const thirdParameterField = await screen.findByLabelText(
      MockTemplateVersionParameter3.name,
      { exact: false },
    );
    expect(thirdParameterField).toBeDefined();
    fireEvent.change(thirdParameterField, {
      target: { value: "1234" },
    });
    fireEvent.submit(thirdParameterField);

    const validationError = await screen.findByText(
      validationPatternNotMatched,
    );
    expect(validationError).toBeInTheDocument();
  });

  it("gitauth: errors if unauthenticated and submits", async () => {
    jest
      .spyOn(API, "getTemplateVersionGitAuth")
      .mockResolvedValueOnce([MockTemplateVersionGitAuth]);

    renderCreateWorkspacePage();
    await waitForLoaderToBeRemoved();

    const nameField = await screen.findByLabelText(nameLabelText);

    // have to use fireEvent b/c userEvent isn't cleaning up properly between tests
    fireEvent.change(nameField, {
      target: { value: "test" },
    });

    const submitButton = screen.getByText(createWorkspaceText);
    await userEvent.click(submitButton);

    await screen.findByText("You must authenticate to create a workspace!");
  });

  it("auto create a workspace if uses mode=auto", async () => {
    const param = "first_parameter";
    const paramValue = "It works!";
    const createWorkspaceSpy = jest.spyOn(API, "createWorkspace");

    renderWithAuth(<CreateWorkspacePage />, {
      route:
        "/templates/" +
        MockTemplate.name +
        `/workspace?param.${param}=${paramValue}&mode=auto`,
      path: "/templates/:template/workspace",
    });

    await waitFor(() => {
      expect(createWorkspaceSpy).toBeCalledWith(
        MockOrganization.id,
        "me",
        expect.objectContaining({
          template_id: MockTemplate.id,
          rich_parameter_values: [{ name: param, value: paramValue }],
        }),
      );
    });
  });
});
