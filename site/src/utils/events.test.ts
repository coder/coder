import { dispatchCustomEvent, isCustomEvent } from "./events";

describe("events", () => {
  describe("dispatchCustomEvent", () => {
    it("dispatch a custom event", (done) => {
      const eventDetail = { title: "Event title" };

      window.addEventListener("eventType", (event) => {
        if (isCustomEvent(event)) {
          expect(event.detail).toEqual(eventDetail);
          done();
        }
      });

      dispatchCustomEvent("eventType", eventDetail);
    });
  });
});
