import {
  displayError,
  displaySuccess,
  isNotificationTextPrefixed,
  MsgType,
  type NotificationMsg,
  type NotificationTextPrefixed,
  SnackbarEventType,
} from "./utils";

describe("Snackbar", () => {
  describe("isNotificationTextPrefixed", () => {
    // Regression test for case found in #10436
    it("does not crash on null values", () => {
      // Given
      const msg = null;

      // When
      const isTextPrefixed = isNotificationTextPrefixed(msg);

      // Then
      expect(isTextPrefixed).toBe(false);
    });
    it("returns true if prefixed", () => {
      // Given
      const msg: NotificationTextPrefixed = {
        prefix: "warning",
        text: "careful with this workspace",
      };

      // When
      const isTextPrefixed = isNotificationTextPrefixed(msg);

      // Then
      expect(isTextPrefixed).toBe(true);
    });
    it("returns false if not prefixed", () => {
      // Given
      const msg = "plain ol' message";

      // When
      const isTextPrefixed = isNotificationTextPrefixed(msg);

      // Then
      expect(isTextPrefixed).toBe(false);
    });
  });

  describe("displaySuccess", () => {
    const originalWindowDispatchEvent = window.dispatchEvent;
    type TDispatchEventMock = jest.MockedFunction<
      (msg: CustomEvent<NotificationMsg>) => boolean
    >;
    let dispatchEventMock: TDispatchEventMock;

    // Helper function to extract the notification event
    // that was sent to `dispatchEvent`. This lets us validate
    // the contents of the notification event are what we expect.
    const extractNotificationEvent = (
      dispatchEventMock: TDispatchEventMock,
    ): NotificationMsg => {
      // calls[0] is the first call made to the mock (this is reset in `beforeEach`)
      // calls[0][0] is the first argument of the first call
      // calls[0][0].detail is the 'detail' argument passed to the `CustomEvent` -
      // this is the `NotificationMsg` object that gets sent to `dispatchEvent`
      return dispatchEventMock.mock.calls[0][0].detail;
    };

    beforeEach(() => {
      dispatchEventMock = jest.fn();
      window.dispatchEvent =
        dispatchEventMock as unknown as typeof window.dispatchEvent;
    });

    afterEach(() => {
      window.dispatchEvent = originalWindowDispatchEvent;
    });

    it("can be called with only a title", () => {
      // Given
      const expected: NotificationMsg = {
        msgType: MsgType.Success,
        msg: "Test",
        additionalMsgs: undefined,
      };

      // When
      displaySuccess("Test");

      // Then
      expect(dispatchEventMock).toBeCalledTimes(1);
      expect(extractNotificationEvent(dispatchEventMock)).toStrictEqual(
        expected,
      );
    });

    it("can be called with a title and additional message", () => {
      // Given
      const expected: NotificationMsg = {
        msgType: MsgType.Success,
        msg: "Test",
        additionalMsgs: ["additional message"],
      };

      // When
      displaySuccess("Test", "additional message");

      // Then
      expect(dispatchEventMock).toBeCalledTimes(1);
      expect(extractNotificationEvent(dispatchEventMock)).toStrictEqual(
        expected,
      );
    });
  });

  describe("displayError", () => {
    it("shows the title and the message", (done) => {
      const message = "Some error happened";

      window.addEventListener(SnackbarEventType, (event) => {
        const notificationEvent = event as CustomEvent<NotificationMsg>;
        expect(notificationEvent.detail.msg).toEqual(message);
        done();
      });

      displayError(message);
    });
  });
});
