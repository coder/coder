import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { CreateTokenPage } from "./CreateTokenPage";
import * as API from "api/api";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

describe("TokenPage", () => {
  it("shows the success modal", async () => {
    jest.spyOn(API, "createToken").mockResolvedValueOnce({
      key: "abcd",
    });

    // When
    const { container } = renderWithAuth(<CreateTokenPage />, {
      route: "/settings/tokens/new",
      path: "/settings/tokens/new",
    });
    await waitForLoaderToBeRemoved();

    const form = container.querySelector("form") as HTMLFormElement;
    await userEvent.type(screen.getByLabelText(/Name/), "my-token");
    await userEvent.click(
      within(form).getByRole("button", { name: /create token/i }),
    );

    // Then
    expect(screen.getByText("abcd")).toBeInTheDocument();
  });
});
