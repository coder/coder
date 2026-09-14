import { AxiosError, type AxiosResponse } from "axios";
import { newRetryDelay } from "./GitDeviceAuth";

test("device auth retry delay", async () => {
	const slowDownError = new AxiosError(
		"slow_down",
		"500",
		undefined,
		undefined,
		{
			data: {
				detail: "slow_down",
			},
		} as AxiosResponse,
	);
	const retryDelay = newRetryDelay(undefined);

	// If no initial interval is provided, the default must be 5 seconds.
	expect(retryDelay(0, undefined)).toBe(5000);
	// If the error is a slow down error, the interval should increase by 5 seconds
	// for this and all subsequent requests, and by 5 seconds extra delay for this
	// request.
	expect(retryDelay(1, slowDownError)).toBe(15000);
	expect(retryDelay(1, slowDownError)).toBe(15000);
	expect(retryDelay(2, undefined)).toBe(10000);

	// Like previous request.
	expect(retryDelay(3, slowDownError)).toBe(20000);
	expect(retryDelay(3, undefined)).toBe(15000);
	// If the error is not a slow down error, the interval should not increase.
	expect(retryDelay(4, new AxiosError("other", "500"))).toBe(15000);

	// If the initial interval is provided, it should be used.
	const retryDelayWithInitialInterval = newRetryDelay(1);
	expect(retryDelayWithInitialInterval(0, undefined)).toBe(1000);
	expect(retryDelayWithInitialInterval(1, slowDownError)).toBe(11000);
	expect(retryDelayWithInitialInterval(2, undefined)).toBe(6000);
});
