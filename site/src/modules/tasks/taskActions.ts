import type { TaskStatus } from "api/typesGenerated";

/**
 * Task statuses that allow pausing.
 */
const pauseStatuses: TaskStatus[] = [
	"active",
	"initializing",
	"pending",
	"error",
	"unknown",
];

/**
 * Task statuses where the pause button should be disabled (in transition).
 */
const pauseDisabledStatuses: TaskStatus[] = ["pending", "initializing"];

/**
 * Task statuses that allow resuming.
 */
const resumeStatuses: TaskStatus[] = ["paused", "error", "unknown"];

/**
 * Checks if a task can be paused based on its status.
 */
export function canPauseTask(status: TaskStatus): boolean {
	return pauseStatuses.includes(status);
}

/**
 * Checks if the pause action should be disabled for a task status.
 */
export function isPauseDisabled(status: TaskStatus): boolean {
	return pauseDisabledStatuses.includes(status);
}

/**
 * Checks if a task can be resumed based on its status.
 */
export function canResumeTask(status: TaskStatus): boolean {
	return resumeStatuses.includes(status);
}
