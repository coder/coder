import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import TemplateVersionPage from "./TemplateVersionPage"
import * as templateVersionUtils from "util/templateVersion"
import { screen } from "@testing-library/react"
import * as CreateDayString from "util/createDayString"
import userEvent from "@testing-library/user-event"

const TEMPLATE_NAME = "coder-ts"
const VERSION_NAME = "12345"
const TERRAFORM_FILENAME = "main.tf"
const README_FILENAME = "readme.md"
const TEMPLATE_VERSION_FILES = {
  [TERRAFORM_FILENAME]: "{}",
  [README_FILENAME]: "Readme",
}

const setup = async () => {
  jest
    .spyOn(templateVersionUtils, "getTemplateVersionFiles")
    .mockResolvedValueOnce(TEMPLATE_VERSION_FILES)

  jest
    .spyOn(CreateDayString, "createDayString")
    .mockImplementation(() => "a minute ago")

  renderWithAuth(<TemplateVersionPage />, {
    route: `/templates/${TEMPLATE_NAME}/versions/${VERSION_NAME}`,
    path: "/templates/:template/versions/:version",
  })
  await waitForLoaderToBeRemoved()
}

describe("TemplateVersionPage", () => {
  beforeEach(setup)

  it("shows files", () => {
    expect(screen.queryByText(TERRAFORM_FILENAME)).toBeInTheDocument()
    expect(screen.queryByText(README_FILENAME)).toBeInTheDocument()
  })

  it("shows the right content when click on the file name", async () => {
    await userEvent.click(screen.getByText(README_FILENAME))
    expect(
      screen.queryByText(TEMPLATE_VERSION_FILES[README_FILENAME]),
    ).toBeInTheDocument()
  })
})
