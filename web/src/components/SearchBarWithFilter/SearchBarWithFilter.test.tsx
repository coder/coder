import { fireEvent, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { render } from "../../testHelpers/renderHelpers"
import { SearchBarWithFilter } from "./SearchBarWithFilter"

// mock the debounce utility
jest.mock("just-debounce-it", () =>
  jest.fn((fn) => {
    fn.cancel = jest.fn()
    return fn
  }),
)

describe("SearchBarWithFilter", () => {
  it("calls the onFilter handler on keystroke", async () => {
    // When
    const onFilter = jest.fn()
    render(<SearchBarWithFilter onFilter={onFilter} />)

    const searchInput = screen.getByRole("textbox")
    await userEvent.type(searchInput, "workspace") // 9 characters

    // Then
    expect(onFilter).toBeCalledTimes(10) // 9 characters + 1 on component mount
  })

  it("calls the onFilter handler on submit", async () => {
    // When
    const onFilter = jest.fn()
    render(<SearchBarWithFilter onFilter={onFilter} />)

    const searchInput = screen.getByRole("textbox")
    await fireEvent.keyDown(searchInput, { key: "Enter", code: "Enter", charCode: 13 })

    // Then
    expect(onFilter).toBeCalledTimes(1)
  })
})
