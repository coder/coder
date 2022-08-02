import { fireEvent, render } from "@testing-library/react"
import { FC } from "react"
import { act } from "react-dom/test-utils"
import { WrapperComponent } from "../../testHelpers/renderHelpers"
import { ConfirmDialog, ConfirmDialogProps } from "./ConfirmDialog"

namespace Helpers {
  export const Component: FC<React.PropsWithChildren<ConfirmDialogProps>> = (props: ConfirmDialogProps) => {
    return (
      <WrapperComponent>
        <ConfirmDialog {...props} />
      </WrapperComponent>
    )
  }
}

describe("ConfirmDialog", () => {
  it("renders", () => {
    // Given
    const onCloseMock = jest.fn()
    const props = {
      onClose: onCloseMock,
      open: true,
      title: "Test",
    }

    // When
    const { getByRole } = render(<Helpers.Component {...props} />)

    // Then
    expect(getByRole("dialog")).toBeDefined()
  })

  it("does not display cancel for info dialogs", () => {
    // Given (note that info is the default)
    const onCloseMock = jest.fn()
    const props = {
      cancelText: "CANCEL",
      onClose: onCloseMock,
      open: true,
      title: "Test",
    }

    // When
    const { queryByText } = render(<Helpers.Component {...props} />)

    // Then
    expect(queryByText("CANCEL")).toBeNull()
  })

  it("can display cancel when normally hidden", () => {
    // Given
    const onCloseMock = jest.fn()
    const props = {
      cancelText: "CANCEL",
      onClose: onCloseMock,
      open: true,
      title: "Test",
      hideCancel: false,
    }

    // When
    const { getByText } = render(<Helpers.Component {...props} />)

    // Then
    expect(getByText("CANCEL")).toBeDefined()
  })

  it("displays cancel for delete dialogs", () => {
    // Given
    const onCloseMock = jest.fn()
    const props: ConfirmDialogProps = {
      cancelText: "CANCEL",
      onClose: onCloseMock,
      open: true,
      title: "Test",
      type: "delete",
    }

    // When
    const { getByText } = render(<Helpers.Component {...props} />)

    // Then
    expect(getByText("CANCEL")).toBeDefined()
  })

  it("can hide cancel when normally visible", () => {
    // Given
    const onCloseMock = jest.fn()
    const props: ConfirmDialogProps = {
      cancelText: "CANCEL",
      onClose: onCloseMock,
      open: true,
      title: "Test",
      hideCancel: true,
      type: "delete",
    }

    // When
    const { queryByText } = render(<Helpers.Component {...props} />)

    // Then
    expect(queryByText("CANCEL")).toBeNull()
  })

  it("onClose is called when cancelled", () => {
    // Given
    const onCloseMock = jest.fn()
    const props = {
      cancelText: "CANCEL",
      hideCancel: false,
      onClose: onCloseMock,
      open: true,
      title: "Test",
    }

    // When
    const { getByText } = render(<Helpers.Component {...props} />)
    act(() => {
      fireEvent.click(getByText("CANCEL"))
    })

    // Then
    expect(onCloseMock).toBeCalledTimes(1)
  })

  it("onConfirm is called when confirmed", () => {
    // Given
    const onCloseMock = jest.fn()
    const onConfirmMock = jest.fn()
    const props = {
      cancelText: "CANCEL",
      confirmText: "CONFIRM",
      hideCancel: false,
      onClose: onCloseMock,
      onConfirm: onConfirmMock,
      open: true,
      title: "Test",
    }

    // When
    const { getByText } = render(<Helpers.Component {...props} />)
    act(() => {
      fireEvent.click(getByText("CONFIRM"))
    })

    // Then
    expect(onCloseMock).toBeCalledTimes(0)
    expect(onConfirmMock).toBeCalledTimes(1)
  })
})
