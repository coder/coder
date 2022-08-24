import { fireEvent, render, screen } from "@testing-library/react"
import { SplitButton, SplitButtonProps } from "./SplitButton"

namespace Helpers {
  export type SplitButtonOptions = "a" | "b" | "c"

  // eslint-disable-next-line @typescript-eslint/no-empty-function, @typescript-eslint/no-unused-vars
  export const callback = (selectedOption: SplitButtonOptions): void => {}

  export const options: SplitButtonProps<SplitButtonOptions>["options"] = [
    {
      label: "test a",
      value: "a",
    },
    {
      label: "test b",
      value: "b",
    },
    {
      label: "test c",
      value: "c",
    },
  ]
}

describe("SplitButton", () => {
  describe("onClick", () => {
    it("is called when primary action is clicked", () => {
      // Given
      const mockedAndSpyedCallback = jest.fn(Helpers.callback)

      // When
      render(<SplitButton onClick={mockedAndSpyedCallback} options={Helpers.options} />)
      fireEvent.click(screen.getByText("test a"))

      // Then
      expect(mockedAndSpyedCallback.mock.calls.length).toBe(1)
      expect(mockedAndSpyedCallback.mock.calls[0][0]).toBe("a")
    })

    it("is called when clicking option in pop-up", () => {
      // Given
      const mockedAndSpyedCallback = jest.fn(Helpers.callback)

      // When
      render(<SplitButton onClick={mockedAndSpyedCallback} options={Helpers.options} />)
      const buttons = screen.getAllByRole("button")
      const dropdownButton = buttons[1]
      fireEvent.click(dropdownButton)
      fireEvent.click(screen.getByText("test c"))

      // Then
      expect(mockedAndSpyedCallback.mock.calls.length).toBe(1)
      expect(mockedAndSpyedCallback.mock.calls[0][0]).toBe("c")
    })
  })
})
