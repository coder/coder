import { fireEvent, screen } from "@testing-library/react"
import { renderWithAuth } from "testHelpers/renderHelpers"
import { AccountPage } from "pages/UserSettingsPage/AccountPage/AccountPage"
import i18next from "i18next"

const { t } = i18next

describe("NavbarLayout", () => {
  it("sets localStorage key-value when dismissed", async () => {
    const localStorageMock = {
      ...global.localStorage,
      getItem: jest.fn(),
    }
    global.localStorage = localStorageMock

    // rendering a random page that is wrapped in NavbarLayout
    renderWithAuth(<AccountPage />)

    fireEvent.click(
      screen.getByRole("button", {
        name: t("ctas.dismissCta", { ns: "common" }),
      }),
    )

    expect(localStorageMock.getItem).toHaveBeenCalledWith("dismissedVersion")
  })
})
