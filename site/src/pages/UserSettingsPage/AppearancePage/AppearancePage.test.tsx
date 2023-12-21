import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as API from "api/api";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { AppearancePage } from "./AppearancePage";
import { MockUser } from "testHelpers/entities";

describe("appearance page", () => {
  it("does nothing when selecting current theme", async () => {
    renderWithAuth(<AppearancePage />);

    jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
      ...MockUser,
      theme_preference: "dark",
    });

    const dark = await screen.findByText("Dark");
    await userEvent.click(dark);

    // Check if the API was called correctly
    expect(API.updateAppearanceSettings).toBeCalledTimes(0);
  });

  it("changes theme to dark blue", async () => {
    renderWithAuth(<AppearancePage />);

    jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
      ...MockUser,
      theme_preference: "darkBlue",
    });

    const darkBlue = await screen.findByText("Dark blue");
    await userEvent.click(darkBlue);

    // Check if the API was called correctly
    expect(API.updateAppearanceSettings).toBeCalledTimes(1);
    expect(API.updateAppearanceSettings).toHaveBeenCalledWith("me", {
      theme_preference: "darkBlue",
    });
  });

  it("changes theme to light", async () => {
    renderWithAuth(<AppearancePage />);

    jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
      ...MockUser,
      theme_preference: "light",
    });

    const light = await screen.findByText("Light");
    await userEvent.click(light);

    // Check if the API was called correctly
    expect(API.updateAppearanceSettings).toBeCalledTimes(1);
    expect(API.updateAppearanceSettings).toHaveBeenCalledWith("me", {
      theme_preference: "light",
    });
  });
});
