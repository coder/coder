import { API } from "api/api";
import { useCallback } from "react";
import type { Option } from "components/MultiSelectCombobox/MultiSelectCombobox";
import {
	ALLOW_LIST_QUICK_PICK_GROUP,
	ALLOW_LIST_SPECIFIC_TEMPLATE_ACTION,
	ALLOW_LIST_SPECIFIC_WORKSPACE_ACTION,
} from "./allowListOptions";

const pluralizeResourceType = (type: string) => {
	if (type === "" || type === "*") {
		return type;
	}
	return type.endsWith("s") ? type : `${type}s`;
};

const formatAllowListOptionLabel = (value: string, label: string) => {
	const trimmedLabel = label.trim();
	if (trimmedLabel === "") {
		return label;
	}
	const [typePart = ""] = value.split(":", 1);
	const type = typePart.trim();
	if (type === "" || type === "*") {
		return trimmedLabel;
	}
	const lowercaseLabel = trimmedLabel.toLowerCase();
	const prefix = `${type} :`;
	if (lowercaseLabel.startsWith(prefix.toLowerCase())) {
		return trimmedLabel;
	}
	return `${type} : ${trimmedLabel}`;
};

export const useAllowListResolver = () => {
	return useCallback(async (input: string): Promise<Option[]> => {
		const trimmed = input.trim();
		const wantsGlobalWildcard =
			trimmed === "" || trimmed === "*" || trimmed === "*:*";
		const [rawType, rawSearch = ""] = trimmed.includes(":")
			? trimmed.split(":", 2)
			: ["", trimmed];
		const normalizedType = rawType.trim().toLowerCase();
		const search = rawSearch.trim();
		const fallbackNeedle = trimmed;

		const matchesNeedle = (option: Option, needle: string) => {
			if (needle === "") {
				return true;
			}
			const lowered = needle.toLowerCase();
			return (
				option.value.toLowerCase().includes(lowered) ||
				option.label.toLowerCase().includes(lowered)
			);
		};

		const aggregator = new Map<string, Option>();
		const addOption = (option: Option, needle: string) => {
			if (!matchesNeedle(option, needle)) {
				return;
			}
			if (!aggregator.has(option.value)) {
				aggregator.set(option.value, option);
			}
		};

		if (wantsGlobalWildcard) {
			addOption(
				{ value: "*:*", label: "Any resource", group: "Wildcard" },
				"",
			);
			addOption(
				{
					value: ALLOW_LIST_SPECIFIC_WORKSPACE_ACTION,
					label: formatAllowListOptionLabel(
						"workspace:",
						"Specific workspace…",
					),
					group: ALLOW_LIST_QUICK_PICK_GROUP,
					prefix: "workspace:",
				},
				"",
			);
			addOption(
				{
					value: ALLOW_LIST_SPECIFIC_TEMPLATE_ACTION,
					label: formatAllowListOptionLabel(
						"template:",
						"Specific template…",
					),
					group: ALLOW_LIST_QUICK_PICK_GROUP,
					prefix: "template:",
				},
				"",
			);
		}

		const shouldQueryWorkspaces =
			normalizedType === "" || normalizedType === "workspace";
		const shouldQueryTemplates =
			normalizedType === "" || normalizedType === "template";

		if (shouldQueryWorkspaces) {
			const needle =
				normalizedType === "workspace" ? search : fallbackNeedle;
			if (needle === "") {
				addOption(
					{
						value: "workspace:*",
						label: formatAllowListOptionLabel(
							"workspace:*",
							"All workspaces",
						),
						group: pluralizeResourceType("workspace"),
					},
					needle,
				);
			}
			if (needle !== "") {
				const request: { limit: number; q?: string } = { limit: 20 };
				request.q = needle;
				try {
					const { workspaces } = await API.getWorkspaces(request);
					for (const workspace of workspaces) {
						addOption(
							{
								value: `workspace:${workspace.id}`,
								label: formatAllowListOptionLabel(
									`workspace:${workspace.id}`,
									workspace.name?.trim() || workspace.id,
								),
								group: pluralizeResourceType("workspace"),
								description: workspace.owner_name
									? `Owner: ${workspace.owner_name}`
									: undefined,
							},
							needle,
						);
					}
				} catch (error) {
					console.error("Failed to resolve workspaces for allow-list", error);
				}
			}
		}

		if (shouldQueryTemplates) {
			const needle =
				normalizedType === "template" ? search : fallbackNeedle;
			if (needle === "") {
				addOption(
					{
						value: "template:*",
						label: formatAllowListOptionLabel(
							"template:*",
							"All templates",
						),
						group: pluralizeResourceType("template"),
					},
					needle,
				);
			}
			if (needle !== "") {
				try {
					const templates = await API.getTemplates({ q: needle });
					for (const template of templates.slice(0, 20)) {
						addOption(
							{
								value: `template:${template.id}`,
								label: formatAllowListOptionLabel(
									`template:${template.id}`,
									template.display_name?.trim() || template.name,
								),
								group: pluralizeResourceType("template"),
							},
							needle,
						);
					}
				} catch (error) {
					console.error("Failed to resolve templates for allow-list", error);
				}
			}
		}

		return [...aggregator.values()];
	}, []);
};
