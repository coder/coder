import { dispatchCustomEvent, isCustomEvent } from "./events";

describe("events", () => {
	describe("dispatchCustomEvent", () => {
		it("dispatch a custom event", () => {
			const eventDetail = { title: "Event title" };

			return new Promise<void>((resolve) => {
				window.addEventListener("eventType", (event) => {
					if (isCustomEvent(event)) {
						expect(event.detail).toEqual(eventDetail);
						resolve();
					}
				});

				dispatchCustomEvent("eventType", eventDetail);
			});
		});
	});
});
