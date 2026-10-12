/**
 * The automations page route and the chat-automations experiment flag that
 * gates the page and its entry points.
 */
import { useDashboard } from "#/modules/dashboard/useDashboard";

export const AUTOMATIONS_PATH = "/agents/automations";

export const newAutomationPath = `${AUTOMATIONS_PATH}/new`;

export const automationPath = (automationId: string) =>
	`${AUTOMATIONS_PATH}/${encodeURIComponent(automationId)}`;

/** Whether the chat-automations experiment is on for the viewer. */
export const useAutomationsEnabled = (): boolean =>
	useDashboard().experiments.includes("chat-automations");
