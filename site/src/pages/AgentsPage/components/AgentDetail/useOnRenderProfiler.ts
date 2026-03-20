import { type ProfilerOnRenderCallback, useCallback, useRef } from "react";

// Threshold in milliseconds. Renders exceeding one frame budget
// (16.67ms at 60fps) are logged as warnings.
const SLOW_RENDER_THRESHOLD_MS = 16;

// Minimum interval between consecutive warnings for the same profiler
// id, to avoid flooding the console during rapid streaming updates.
const WARN_THROTTLE_MS = 2000;

/**
 * Returns a stable onRender callback for React's <Profiler> component.
 * When a render exceeds SLOW_RENDER_THRESHOLD_MS, it logs a
 * console.warn with timing details and emits a performance.measure()
 * entry visible in browser devtools (including Safari Timeline).
 *
 * The <Profiler> onRender callback is a no-op in standard production
 * builds. It only receives timing data when the app is built with
 * react-dom/profiling (enabled via CODER_REACT_PROFILING=true).
 */
export function useOnRenderProfiler(): ProfilerOnRenderCallback {
	const lastWarnTime = useRef<Record<string, number>>({});

	return useCallback<ProfilerOnRenderCallback>(
		(id, phase, actualDuration, baseDuration, startTime, commitTime) => {
			// Emit a performance.measure entry for every render so it
			// appears in the browser's Performance/Timeline panel
			// regardless of whether it's "slow". The entry name uses a
			// React atom symbol to make it easy to filter.
			try {
				performance.measure(`⚛ ${id} (${phase})`, {
					start: startTime,
					duration: actualDuration,
				});
			} catch {
				// performance.measure can throw if startTime is invalid
				// (e.g. negative or before time origin). Safe to ignore.
			}

			if (actualDuration <= SLOW_RENDER_THRESHOLD_MS) {
				return;
			}

			const now = performance.now();
			const last = lastWarnTime.current[id] ?? 0;
			if (now - last < WARN_THROTTLE_MS) {
				return;
			}
			lastWarnTime.current[id] = now;

			console.warn(
				`[Slow render] %c${id}%c ${phase}: ` +
					`${actualDuration.toFixed(1)}ms actual, ` +
					`${baseDuration.toFixed(1)}ms base ` +
					`(commit ${(commitTime - startTime).toFixed(1)}ms after start)`,
				"font-weight: bold",
				"font-weight: normal",
			);
		},
		[],
	);
}
