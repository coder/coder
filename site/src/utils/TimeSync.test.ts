import { TimeSync } from "./TimeSync";

describe(TimeSync.name, () => {
	describe("Subscriptions", () => {
		it("Lets an external system subscribe to periodic time updates", () => {
			expect.hasAssertions();
		});

		it("Throws an error if provided subscription interval is not a positive integer", () => {
			expect.hasAssertions();
		});

		it("Only calls onUpdate callback once if subscription is registered multiple times for the same time interval", () => {
			expect.hasAssertions();
		});

		it("Only calls onUpdate callback if subscription is registered multiple times for different time intervals", () => {
			expect.hasAssertions();
		});

		it("Only calls onUpdate callback if subscription is registered with a mix of redundant/different intervals", () => {
			expect.hasAssertions();
		});

		it("Lets any external system manually flush the latest state snapshot to all subscribers", () => {
			expect.hasAssertions();
		});
	});

	describe("Additional public methods", () => {
		it("Lets any external system access the latest snapshot WITHOUT subscribing first", () => {
			expect.hasAssertions();
		});
	});
});
