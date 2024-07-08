import { screen, within } from "@testing-library/react";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import * as CreateDayString from "utils/createDayString";
import * as templateVersionUtils from "utils/templateVersion";
import TemplateVersionPage from "./TemplateVersionPage";

const TEMPLATE_NAME = "coder-ts";
const VERSION_NAME = "12345";
const TERRAFORM_FILENAME = "main.tf";
const README_FILENAME = "readme.md";
const TEMPLATE_VERSION_FILES = {
  [TERRAFORM_FILENAME]: "{}",
  [README_FILENAME]: "Readme",
};

const setup = async () => {
  jest
    .spyOn(templateVersionUtils, "getTemplateVersionFiles")
    .mockResolvedValue(TEMPLATE_VERSION_FILES);

  jest
    .spyOn(CreateDayString, "createDayString")
    .mockImplementation(() => "a minute ago");

  renderWithAuth(<TemplateVersionPage />, {
    route: `/templates/${TEMPLATE_NAME}/versions/${VERSION_NAME}`,
    path: "/templates/:template/versions/:version",
  });
  await waitForLoaderToBeRemoved();
};

describe("TemplateVersionPage", () => {
  beforeEach(setup);

  it("shows files", () => {
    const files = screen.getByTestId("template-files-content");
    expect(within(files).getByText(TERRAFORM_FILENAME)).toBeInTheDocument();
    expect(within(files).getByText(README_FILENAME)).toBeInTheDocument();
  });
});
