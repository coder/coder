import { renderHook, waitFor } from "@testing-library/react";
import { dispatchCustomEvent } from "utils/events";
import { useCustomEvent } from "./events";

describe(useCustomEvent.name, () => {
  it("Should receive custom events dispatched by the dispatchCustomEvent function", async () => {
    const mockCallback = jest.fn();
    const eventType = "testEvent";
    const detail = { title: "We have a new event!" };

    renderHook(() => useCustomEvent(eventType, mockCallback));
    dispatchCustomEvent(eventType, detail);

    await waitFor(() => expect(mockCallback).toBeCalledTimes(1));
    expect(mockCallback.mock.calls[0]?.[0]?.detail).toBe(detail);
  });
});
