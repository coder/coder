export const delay = (ms: number, signal?: AbortSignal): Promise<void> =>
	new Promise((res) => {
		if (signal?.aborted) {
			res();
			return;
		}
		const onAbort = () => {
			clearTimeout(timeoutId);
			res();
		};
		const timeoutId = setTimeout(() => {
			signal?.removeEventListener("abort", onAbort);
			res();
		}, ms);
		signal?.addEventListener("abort", onAbort, { once: true });
	});
