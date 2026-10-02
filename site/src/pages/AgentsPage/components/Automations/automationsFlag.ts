/**
 * Automations are an experiment. Every surface that links to or renders the
 * automations page checks this flag, so the page stays hidden until the
 * deployment enables the chat-automations experiment.
 */
import { useDashboard } from "#/modules/dashboard/useDashboard";

export const AUTOMATIONS_PATH = "/agents/automations";

/** Whether the chat-automations experiment is on for the viewer. */
export const useAutomationsEnabled = (): boolean =>
	useDashboard().experiments.includes("chat-automations");
