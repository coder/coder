import { fireEvent, render, screen } from "@testing-library/react"
import { FormCloseButton } from "./FormCloseButton"

describe("FormCloseButton", () => {
  it("renders", async () => {
    // When
    render(
      <FormCloseButton
        onClose={() => {
          return
        }}
      />,
    )

    // Then
    await screen.findByText("ESC")
  })

  it("calls onClose when clicked", async () => {
    // Given
    const onClose = jest.fn()

    // When
    render(<FormCloseButton onClose={onClose} />)

    // Then
    const element = await screen.findByText("ESC")

    // When
    fireEvent.click(element)

    // Then
    expect(onClose).toBeCalledTimes(1)
  })

  it("calls onClose when escape is pressed", async () => {
    // Given
    const onClose = jest.fn()

    // When
    render(<FormCloseButton onClose={onClose} />)

    // Then
    const element = await screen.findByText("ESC")

    // When
    fireEvent.keyDown(element, { key: "Escape", code: "Esc", charCode: 27 })

    // Then
    expect(onClose).toBeCalledTimes(1)
  })

  it("doesn't call onClose if another key is pressed", async () => {
    // Given
    const onClose = jest.fn()

    // When
    render(<FormCloseButton onClose={onClose} />)

    // Then
    const element = await screen.findByText("ESC")

    // When
    fireEvent.keyDown(element, { key: "Enter", code: "Enter", charCode: 13 })

    // Then
    expect(onClose).toBeCalledTimes(0)
  })
})
