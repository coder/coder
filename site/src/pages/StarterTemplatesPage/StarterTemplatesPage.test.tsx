import { screen } from "@testing-library/react";
import {
  MockTemplateExample,
  MockTemplateExample2,
} from "testHelpers/entities";
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import StarterTemplatesPage from "./StarterTemplatesPage";

describe("StarterTemplatesPage", () => {
  it("shows the starter template", async () => {
    renderWithAuth(<StarterTemplatesPage />, {
      route: `/starter-templates`,
      path: "/starter-templates",
    });
    await waitForLoaderToBeRemoved();
    expect(screen.getByText(MockTemplateExample.name)).toBeInTheDocument();
    expect(screen.getByText(MockTemplateExample2.name)).toBeInTheDocument();
  });
});
