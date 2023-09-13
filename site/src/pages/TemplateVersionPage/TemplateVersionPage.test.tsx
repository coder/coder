import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import TemplateVersionPage from "./TemplateVersionPage";
import * as templateVersionUtils from "utils/templateVersion";
import { screen } from "@testing-library/react";
import * as CreateDayString from "utils/createDayString";

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
    expect(screen.getByText(TERRAFORM_FILENAME)).toBeInTheDocument();
    expect(screen.getByText(README_FILENAME)).toBeInTheDocument();
  });
});
