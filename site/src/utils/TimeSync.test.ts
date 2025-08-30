import { TimeSync } from "./TimeSync";

const sampleInvalidIntervals: readonly number[] = [
	Number.NaN,
	Number.NEGATIVE_INFINITY,
	Number.POSITIVE_INFINITY,
	0,
	-42,
	470.53,
];

// Just doing a bunch of pseodocode tests for now to validate my assumptions
// about how the system should work
describe(TimeSync.name, () => {
	describe("Subscriptions: default behavior", () => {
		it("Does not ever update any internal state while there are zero subscribers", () => {
			expect.hasAssertions();
		});

		it("Lets a single subscriber subscribe to periodic time updates", () => {
			expect.hasAssertions();
		});

		it("Lets multiple subscriber subscribe to periodic time updates", () => {
			expect.hasAssertions();
		});

		// This is really important behavior for the React bindings. Those use
		// useSyncExternalStore under the hood, which require that you always
		// return out the same value by reference every time React tries to pull
		// a value from an external state source
		it("Exposes the exact same date snapshot (by reference) to subscribers on each update", () => {
			expect.hasAssertions();
		});

		it("Throws an error if provided subscription interval is not a positive integer", () => {
			const sync = new TimeSync();
			const dummyFunction = jest.fn();

			for (const i of sampleInvalidIntervals) {
				expect(() => {
					void sync.subscribe({
						targetRefreshIntervalMs: i,
						onUpdate: dummyFunction,
					});
				}).toThrow(
					`TimeSync refresh interval must be a positive integer (received ${i}ms)`,
				);
			}
		});

		it("Dispatches updates to all subscribers based on fastest interval specified", () => {
			expect.hasAssertions();
		});

		it("Calls onUpdate callback one time total if subscription is registered multiple times for the same time interval", () => {
			expect.hasAssertions();
		});

		it("Calls onUpdate callback one time total if subscription is registered multiple times for different time intervals", () => {
			expect.hasAssertions();
		});

		it("Calls onUpdate callback one time total if subscription is registered multiple times with a mix of redundant/different intervals", () => {
			expect.hasAssertions();
		});

		it("Lets an external system unsubscribe", () => {
			expect.hasAssertions();
		});

		it("Slows updates down to the second-fastest interval when the all subscribers for the fastest interval unsubscribe", () => {
			expect.hasAssertions();
		});

		/**
		 * Was really hard to describe this in a single sentence, but basically:
		 * 1. Let's say that we have subscribers A AND B. A subscribes for 500ms
		 *    and B subscribes for 1000ms.
		 * 2. At 450ms, A unsubscribes.
		 * 3. Rather than starting the timer over, a one-time 'pseudo-timeout'
		 *    is kicked off for the delta between the elapsed time and B (650ms)
		 * 4. After the timeout resolves, updates go back to happening on an
		 *    interval of 1000ms.
		 */
		it("Does not completely start next interval over from scratch if fastest subscription is removed halfway through update", () => {
			expect.hasAssertions();
		});

		it("Immediately notifies subscribers if new refresh interval is added that is less than or equal to the time since the last update", () => {
			expect.hasAssertions();
		});

		it("Does not fully remove an onUpdate callback if multiple systems use it to subscribe, and only one system unsubscribes", () => {
			expect.hasAssertions();
		});

		it("Automatically updates the date snapshot after the very first subscription is received, regardless of specified refresh interval", () => {
			expect.hasAssertions();
		});

		it("Does not ever do periodic notifications if all subscribers specify an update interval of positive infinity", () => {
			expect.hasAssertions();
		});

		it("Never indicates to new subscriber that there are pending updates (even if the subscription updates the date snapshot)", () => {
			expect.hasAssertions();
		});
	});

	describe("Subscriptions: custom `minimumRefreshIntervalMs` value", () => {
		it("Rounds up all incoming subscription intervals to custom min interval", () => {
			expect.hasAssertions();
		});

		it("Throws if custom min interval is not a positive integer", () => {
			for (const i of sampleInvalidIntervals) {
				expect(() => {
					void new TimeSync({ minimumRefreshIntervalMs: i });
				}).toThrow(
					`Minimum refresh interval must be a positive integer (received ${i}ms)`,
				);
			}
		});
	});

	// This behavior is needed to make TimeSync play well with React's
	// lifecycles, but it didn't feel reasonable to make this behavior the
	// default for a system that should ideally be decoupled from React
	describe("Subscriptions: turning `autoNotifyAfterStateUpdate` off", () => {
		it("Does not auto-notify subscribers when date state is updated", () => {
			expect.hasAssertions();
		});

		it("Indicates to new subscriber that there are pending subscribers if the subscription updates the date snapshot", () => {
			expect.hasAssertions();
		});
	});

	describe("Other public methods", () => {
		it("Lets any external system manually flush the latest state snapshot to all subscribers (for any reason, at any time)", () => {
			expect.hasAssertions();
		});

		it("Lets any external system access the latest date snapshot without subscribing", () => {
			expect.hasAssertions();
		});

		it("Keeps pulled date snapshot over time as other subscribers update it", () => {
			expect.hasAssertions();
		});
	});
});
