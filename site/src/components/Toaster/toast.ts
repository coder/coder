import { isValidElement } from "react";
import { type ExternalToast, toast as sonnerToast } from "sonner";

// Error toasts stay until the user dismisses them so everyone has enough time
// to read and act on them (WCAG 2.2.1 Timing Adjustable). Other toast types
// keep Sonner's timer.
const persistentToast = {
	duration: Number.POSITIVE_INFINITY,
} satisfies ExternalToast;

const error: typeof sonnerToast.error = (message, data) =>
	sonnerToast.error(message, { ...persistentToast, ...data });

// Sonner renders a rejected promise as an error toast without calling
// toast.error, so the error result needs the persistent duration too.
const promise: typeof sonnerToast.promise = (promiseToTrack, data) => {
	if (data?.error === undefined) {
		return sonnerToast.promise(promiseToTrack, data);
	}
	const errorResult = data.error;
	return sonnerToast.promise(promiseToTrack, {
		...data,
		error: async (err: unknown) => {
			const result =
				typeof errorResult === "function"
					? await errorResult(err)
					: errorResult;
			if (
				typeof result === "object" &&
				result !== null &&
				!isValidElement(result) &&
				"message" in result
			) {
				return { ...persistentToast, ...result };
			}
			return { ...persistentToast, message: result };
		},
	});
};

export const toast: typeof sonnerToast = Object.assign(
	(...args: Parameters<typeof sonnerToast>) => sonnerToast(...args),
	sonnerToast,
	{ error, promise },
);
