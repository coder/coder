import { ActionCell, ActionCellProps, LANGUAGE } from "./ActionCell"
import React from "react"
import { render, screen } from "@testing-library/react"

namespace Helpers {
  export const Props: ActionCellProps = {
    action: "create",
    statusCode: 200,
  }
  export const Component: React.FC<ActionCellProps> = (props) => <ActionCell {...props} />
}

describe("ActionCellProps", () => {
  it.each<[ActionCellProps, ActionCellProps, boolean]>([
    [{ action: "Create", statusCode: 200 }, { action: "Create", statusCode: 200 }, false],
    [{ action: " Create ", statusCode: 400 }, { action: "Create", statusCode: 400 }, false],
    [{ action: "", statusCode: 200 }, { action: "", statusCode: 200 }, true],
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
  it.each<[number, boolean]>([
    // success cases
    [200, true],
    [201, true],
    [302, true],
    // failure cases
    [400, false],
    [404, false],
    [500, false],
  ])(`isSuccessStatus(%p) returns %p`, (statusCode, expected) => {
    expect(ActionCellProps.isSuccessStatus(statusCode)).toBe(expected)
  })
})

describe("ActionCell", () => {
  // action cases
  it("renders the action", () => {
    // Given
    const props = Helpers.Props

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(props.action)).toBeDefined()
  })
  it("throws when action is an empty string", () => {
    // Given
    const props: ActionCellProps = {
      ...Helpers.Props,
      action: "",
    }

    // When
    const shouldThrow = () => {
      render(<Helpers.Component {...props} />)
    }

    // Then
    expect(shouldThrow).toThrowError()
  })

  // statusCode cases
  it.each<[number, string]>([
    // Success cases
    [200, LANGUAGE.statusCodeSuccess],
    [201, LANGUAGE.statusCodeSuccess],
    [302, LANGUAGE.statusCodeSuccess],
    // Failure cases
    [400, LANGUAGE.statusCodeFail],
    [404, LANGUAGE.statusCodeFail],
    [500, LANGUAGE.statusCodeFail],
  ])("renders %p when statusCode is %p", (statusCode, expected) => {
    // Given
    const props: ActionCellProps = {
      ...Helpers.Props,
      statusCode,
    }

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(expected)).toBeDefined()
  })
})
