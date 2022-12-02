import { fireEvent, screen, waitFor } from "@testing-library/react"
import i18next from "i18next"
import { MockUpdateCheck, render } from "testHelpers/renderHelpers"
import { UpdateCheckBanner } from "./UpdateCheckBanner"

describe("UpdateCheckBanner", () => {
  it("shows an update notification when one is available", () => {
    const { t } = i18next
    render(
      <UpdateCheckBanner
        updateCheck={{ ...MockUpdateCheck, current: false }}
      />,
    )

    const updateText = t("updateCheck.message", {
      ns: "common",
      version: MockUpdateCheck.version,
    })
    // Message contatins HTML elements so we check it in parts.
    for (const text of updateText.split(/<\/?[0-9]+>/)) {
      expect(screen.getByText(text, { exact: false })).toBeInTheDocument()
    }

    expect(screen.getAllByRole("link")[0]).toHaveAttribute(
      "href",
      MockUpdateCheck.url,
    )
  })

  it("is hidden when dismissed", async () => {
    const dismiss = jest.fn()
    const { container } = render(
      <UpdateCheckBanner
        onDismiss={dismiss}
        updateCheck={{ ...MockUpdateCheck, current: false }}
      />,
    )

    fireEvent.click(screen.getByRole("button"))
    await waitFor(() => expect(dismiss).toBeCalledTimes(1), { timeout: 2000 })

    expect(container.firstChild).toBeNull()
  })

  it("does not show when up-to-date", async () => {
    const { container } = render(
      <UpdateCheckBanner updateCheck={{ ...MockUpdateCheck, current: true }} />,
    )
    expect(container.firstChild).toBeNull()
  })
})
