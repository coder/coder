import { type ProfilerOnRenderCallback, useCallback, useRef } from "react";

// Threshold in milliseconds. Renders exceeding one frame budget
// (16.67ms at 60fps) are logged as warnings.
const SLOW_RENDER_THRESHOLD_MS = 16;

// Minimum interval between consecutive warnings for the same profiler
// id, to avoid flooding the console during rapid streaming updates.
const WARN_THROTTLE_MS = 2000;

// Cap the number of performance.measure entries to avoid unbounded
// memory growth during long streaming sessions. When the cap is
// reached, only this profiler's entries are cleared by name
// and counting restarts.
const MAX_MEASURE_ENTRIES = 500;

/**
 * Returns a stable onRender callback for React's <Profiler> component.
 * Every render emits a performance.measure() entry visible in browser
 * devtools (including Safari Timeline). Renders exceeding
 * SLOW_RENDER_THRESHOLD_MS additionally log a console.warn with
 * timing details (throttled per profiler id).
 *
 * In standard production builds, React does not call the onRender
 * callback with timing data, so the hook is effectively inert. It
 * only produces output when built with react-dom/profiling (enabled
 * via CODER_REACT_PROFILING=true).
 */
export function useOnRenderProfiler(): ProfilerOnRenderCallback {
	const lastWarnTime = useRef(0);
	const measureCount = useRef(0);
	const measureNames = useRef(new Set<string>());

	return useCallback<ProfilerOnRenderCallback>(
		(id, phase, actualDuration, baseDuration, startTime, commitTime) => {
			// In standard production builds the Profiler callback
			// receives zero for all timing values. Bail out early to
			// avoid creating garbage performance entries.
			if (actualDuration <= 0) {
				return;
			}

			// Emit a performance.measure entry for every render so
			// the Performance/Timeline panel shows the full render
			// timeline when investigating jank, not just outliers.
			const measureName = `⚛ ${id} (${phase})`;
			try {
				performance.measure(measureName, {
					start: startTime,
					duration: actualDuration,
				});
			} catch {
				// performance.measure can throw if startTime is invalid
				// (e.g. negative or before time origin). Safe to ignore.
			}
			measureNames.current.add(measureName);
			measureCount.current++;
			if (measureCount.current >= MAX_MEASURE_ENTRIES) {
				for (const name of measureNames.current) {
					performance.clearMeasures(name);
				}
				measureNames.current.clear();
				measureCount.current = 0;
			}

			if (actualDuration <= SLOW_RENDER_THRESHOLD_MS) {
				return;
			}

			const now = performance.now();
			if (now - lastWarnTime.current < WARN_THROTTLE_MS) {
				return;
			}
			lastWarnTime.current = now;

			// actualDuration covers the render phase only. The commit
			// offset (commitTime - startTime) includes yield/suspend
			// time in concurrent React, so it can be larger.
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
