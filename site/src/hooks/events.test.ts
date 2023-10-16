import { renderHook, waitFor } from "@testing-library/react";
import { dispatchCustomEvent } from "utils/events";
import { useCustomEvent } from "./events";

describe("useCustomEvent", () => {
  it("should listem a custom event", async () => {
    const callback = jest.fn();
    const detail = { title: "Test event" };
    renderHook(() => useCustomEvent("testEvent", callback));
    dispatchCustomEvent("testEvent", detail);
    await waitFor(() => {
      expect(callback).toBeCalledTimes(1);
    });
    expect(callback.mock.calls[0][0].detail).toBe(detail);
  });
});
