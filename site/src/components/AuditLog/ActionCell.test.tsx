import { ActionCell, ActionCellProps } from "./ActionCell"
import React from "react"
import { render, screen } from "@testing-library/react"

namespace Helpers {
  export const Component: React.FC<ActionCellProps> = (props) => <ActionCell {...props} />
}

describe("ActionCellProps", () => {
  it.each<[ActionCellProps, ActionCellProps, boolean]>([
    [{ action: "Create" }, { action: "Create" }, false],
    [{ action: " Create " }, { action: "Create" }, false],
    [{ action: "" }, { action: "" }, true],
  ])(`validate(%p) throws: %p`, (props, expected, throws) => {
    const validate = () => {
      return ActionCellProps.validate(props)
    }

    if (throws) {
      expect(validate).toThrowError()
    } else {
      expect(validate()).toStrictEqual(expected)
    }
  })
})

describe("ActionCell", () => {
  it("renders the action", () => {
    // Given
    const props: ActionCellProps = {
      action: "Create",
    }

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(props.action)).toBeDefined()
  })

  it("throws when action is an empty string", () => {
    // Given
    const props: ActionCellProps = {
      action: "",
    }

    // When
    const shouldThrow = () => {
      render(<Helpers.Component {...props} />)
    }

    // Then
    expect(shouldThrow).toThrowError()
  })
})
