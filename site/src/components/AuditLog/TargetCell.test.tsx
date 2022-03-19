import React from "react"
import { WrapperComponent } from "../../test_helpers"
import { TargetCell, TargetCellProps, LANGUAGE } from "./TargetCell"
import { fireEvent, render, screen } from "@testing-library/react"

namespace Helpers {
  export const Props: TargetCellProps = {
    name: "name",
    type: "test",
    onSelect: jest.fn(),
  }

  export const Component: React.FC<TargetCellProps> = (props) => (
    <WrapperComponent>
      <TargetCell {...props} />
    </WrapperComponent>
  )
}

describe("TargetCellProps", () => {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  const noop = () => {}

  it.each<[TargetCellProps, TargetCellProps, boolean]>([
    [
      {
        name: "test",
        type: "test",
        onSelect: noop,
      },
      {
        name: "test",
        type: "test",
        onSelect: noop,
      },
      false,
    ],
    [
      {
        name: "",
        type: " test ",
        onSelect: noop,
      },
      {
        name: "",
        type: "test",
        onSelect: noop,
      },
      false,
    ],
    [
      {
        name: "test",
        type: "",
        onSelect: noop,
      },
      {
        name: "test",
        type: "",
        onSelect: noop,
      },
      true,
    ],
  ])(`validate(%p) -> %p throws: %p`, (props, expected, throws) => {
    const validate = () => {
      return TargetCellProps.validate(props)
    }

    if (throws) {
      expect(validate).toThrowError()
    } else {
      expect(validate()).toStrictEqual(expected)
    }
  })
})

describe("TargetCell", () => {
  // onSelect callback
  it("calls onSelect when the name is clicked", () => {
    // Given
    const onSelectMock = jest.fn()
    const props: TargetCellProps = {
      ...Helpers.Props,
      onSelect: onSelectMock,
    }

    // When
    render(<Helpers.Component {...props} />)
    fireEvent.click(screen.getByText(props.name))

    // Then
    expect(onSelectMock).toHaveBeenCalledTimes(1)
  })

  // target name cases
  it("renders a non-empty name", () => {
    // Given
    const props = Helpers.Props

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(props.name)).toBeDefined()
  })
  it(`renders ${LANGUAGE.emptyDisplayName} when name is '""'`, () => {
    // Given
    const props: TargetCellProps = {
      ...Helpers.Props,
      name: "",
    }

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(LANGUAGE.emptyDisplayName)).toBeDefined()
  })

  // target type
  it("renders target type", () => {
    // Given
    const props = Helpers.Props

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(props.type)).toBeDefined()
  })
})
