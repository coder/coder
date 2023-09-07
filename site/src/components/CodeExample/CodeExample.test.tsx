import { screen } from "@testing-library/react";
import { render } from "../../testHelpers/renderHelpers";
import { CodeExample } from "./CodeExample";

describe("CodeExample", () => {
  it("renders code", async () => {
    // When
    render(<CodeExample code="echo hello" />);

    // Then
    // Both lines should be rendered
    await screen.findByText("echo hello");
  });
});
