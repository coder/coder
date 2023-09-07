import { screen } from "@testing-library/react";
import { MockTemplateExample } from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import StarterTemplatePage from "./StarterTemplatePage";

jest.mock("remark-gfm", () => jest.fn());

describe("StarterTemplatePage", () => {
  it("shows the starter template", async () => {
    renderWithAuth(<StarterTemplatePage />, {
      route: `/starter-templates/${MockTemplateExample.id}`,
      path: "/starter-templates/:exampleId",
    });
    await waitForLoaderToBeRemoved();
    expect(screen.getByText(MockTemplateExample.name)).toBeInTheDocument();
  });
});
