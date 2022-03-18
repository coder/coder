import { MockUser, MockUserAgent, WrapperComponent } from "../../test_helpers"
import { LANGUAGE, UserCell, UserCellProps } from "./UserCell"
import React from "react"
import { fireEvent, render, screen } from "@testing-library/react"

namespace Helpers {
  export const Props: UserCellProps = {
    onSelectEmail: jest.fn(),
    user: MockUser,
    userAgent: MockUserAgent,
  }

  export const Component: React.FC<UserCellProps> = (props) => (
    <WrapperComponent>
      <UserCell {...props} />
    </WrapperComponent>
  )
}

describe("UserCell", () => {
  // callbacks
  it("calls onUserClick when an email address is clicked", () => {
    // Given
    const onSelectEmailMock = jest.fn()
    const props: UserCellProps = {
      ...Helpers.Props,
      onSelectEmail: onSelectEmailMock,
    }

    // When - click the user's email address
    render(<Helpers.Component {...props} />)
    fireEvent.click(screen.getByText(props.user.email))

    // Then - callback was fired once
    expect(onSelectEmailMock).toHaveBeenCalledTimes(1)
  })

  // email address cases
  it("renders an existing members' email address", () => {
    // Given
    const props: UserCellProps = Helpers.Props

    // When
    render(<Helpers.Component {...props} />)

    // Then - email address is visible
    expect(screen.getByText(props.user.email)).toBeDefined()
  })
  it(`renders '${LANGUAGE.emptyUser}' for non-existing members`, () => {
    // Given
    const props: UserCellProps = {
      ...Helpers.Props,
      user: {
        ...MockUser,
        email: "",
      },
    }

    // When
    render(<Helpers.Component {...props} />)

    // Then - 'Deleted user' is visible
    expect(screen.getByText(LANGUAGE.emptyUser)).toBeDefined()
  })

  // ip address
  it("renders user agent IP address", () => {
    // Given
    const props: UserCellProps = Helpers.Props

    // When
    render(<Helpers.Component {...props} />)

    // Then - ip address is visible
    expect(screen.getByText(props.userAgent.ip_address)).toBeDefined()
  })
})
