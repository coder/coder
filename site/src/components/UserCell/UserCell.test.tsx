import { fireEvent, render, screen } from "@testing-library/react"
import { FC } from "react"
import { MockUser, MockUserAgent, WrapperComponent } from "../../testHelpers/renderHelpers"
import { UserCell, UserCellProps } from "./UserCell"

namespace Helpers {
  export const Props: UserCellProps = {
    Avatar: {
      username: MockUser.username,
    },
    caption: MockUserAgent.ip_address,
    primaryText: MockUser.username,
    onPrimaryTextSelect: jest.fn(),
  }

  export const Component: FC<React.PropsWithChildren<UserCellProps>> = (props) => (
    <WrapperComponent>
      <UserCell {...props} />
    </WrapperComponent>
  )
}

describe("UserCell", () => {
  // callbacks
  it("calls onPrimaryTextSelect when primaryText is clicked", () => {
    // Given
    const onPrimaryTextSelectMock = jest.fn()
    const props: UserCellProps = {
      ...Helpers.Props,
      onPrimaryTextSelect: onPrimaryTextSelectMock,
    }

    // When - click the user's email address
    render(<Helpers.Component {...props} />)
    fireEvent.click(screen.getByText(props.primaryText))

    // Then - callback was fired once
    expect(onPrimaryTextSelectMock).toHaveBeenCalledTimes(1)
  })

  // primaryText
  it("renders primaryText as a link when onPrimaryTextSelect is defined", () => {
    // Given
    const props: UserCellProps = Helpers.Props

    // When
    render(<Helpers.Component {...props} />)
    const primaryTextNode = screen.getByText(props.primaryText)

    // Then
    expect(primaryTextNode.tagName).toBe("A")
  })
  it("renders primaryText without a link when onPrimaryTextSelect is undefined", () => {
    // Given
    const props: UserCellProps = {
      ...Helpers.Props,
      onPrimaryTextSelect: undefined,
    }

    // When
    render(<Helpers.Component {...props} />)
    const primaryTextNode = screen.getByText(props.primaryText)

    // Then
    expect(primaryTextNode.tagName).toBe("P")
  })

  // caption
  it("renders caption", () => {
    // Given
    const caption = "definitely a caption"
    const props: UserCellProps = {
      ...Helpers.Props,
      caption,
    }

    // When
    render(<Helpers.Component {...props} />)

    // Then
    expect(screen.getByText(caption)).toBeDefined()
  })
})
