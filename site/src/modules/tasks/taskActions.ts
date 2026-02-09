import type { TaskStatus } from "api/typesGenerated";

/**
 * Task statuses that allow pausing.
 */
const PAUSABLE_STATUSES: TaskStatus[] = [
	"active",
	"initializing",
	"pending",
	"error",
	"unknown",
];

/**
 * Task statuses where the pause button should be disabled (in transition).
 */
const PAUSE_DISABLED_STATUSES: TaskStatus[] = ["pending", "initializing"];

/**
 * Task statuses that allow resuming.
 */
const RESUMABLE_STATUSES: TaskStatus[] = ["paused", "error", "unknown"];

/**
 * Checks if a task can be paused based on its status.
 */
export function canPauseTask(status: TaskStatus): boolean {
	return PAUSABLE_STATUSES.includes(status);
}

/**
 * Checks if the pause action should be disabled for a task status.
 */
export function isPauseDisabled(status: TaskStatus): boolean {
	return PAUSE_DISABLED_STATUSES.includes(status);
}

/**
 * Checks if a task can be resumed based on its status.
 */
export function canResumeTask(status: TaskStatus): boolean {
	return RESUMABLE_STATUSES.includes(status);
}
