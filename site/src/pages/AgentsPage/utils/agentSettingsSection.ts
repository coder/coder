const AGENT_SETTINGS_SEARCH_PARAM = "settings";

export const AGENT_SETTINGS_SECTIONS = [
	"general",
	"user-agents",
	"personal-skills",
	"compaction",
	"api-keys",
] as const;

export type AgentSettingsSection = (typeof AGENT_SETTINGS_SECTIONS)[number];

export const DEFAULT_AGENT_SETTINGS_SECTION: AgentSettingsSection = "general";

export const isAgentSettingsSection = (
	value: string | null | undefined,
): value is AgentSettingsSection =>
	AGENT_SETTINGS_SECTIONS.some((section) => section === value);

/**
 * The open settings section, or undefined when the dialog is closed. Unknown
 * slugs fall back to the first section so stale links still open settings.
 */
export const agentSettingsSectionFromSearch = (
	search: URLSearchParams | string,
): AgentSettingsSection | undefined => {
	const params =
		typeof search === "string" ? new URLSearchParams(search) : search;
	const value = params.get(AGENT_SETTINGS_SEARCH_PARAM);
	if (value === null) {
		return undefined;
	}
	return isAgentSettingsSection(value) ? value : DEFAULT_AGENT_SETTINGS_SECTION;
};

const toSearchString = (params: URLSearchParams): string => {
	const search = params.toString();
	return search === "" ? "" : `?${search}`;
};

export const withAgentSettingsSection = (
	search: URLSearchParams | string,
	section: AgentSettingsSection,
): string => {
	const params = new URLSearchParams(search);
	params.set(AGENT_SETTINGS_SEARCH_PARAM, section);
	return toSearchString(params);
};

export const withoutAgentSettingsSection = (
	search: URLSearchParams | string,
): string => {
	const params = new URLSearchParams(search);
	params.delete(AGENT_SETTINGS_SEARCH_PARAM);
	return toSearchString(params);
};
