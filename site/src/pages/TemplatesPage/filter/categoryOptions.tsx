import { BanIcon, BotIcon, MonitorIcon, SparklesIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Organization } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";

type AttributeDefinition = {
	label: string;
	value: string;
	icon: ReactNode;
};

const attributeIcon = (icon: ReactNode): ReactNode => (
	<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
		{icon}
	</span>
);

const ATTRIBUTE_DEFINITIONS: readonly AttributeDefinition[] = [
	{
		label: "Deprecated",
		value: "deprecated",
		icon: <BanIcon className="size-icon-sm" />,
	},
	{
		label: "Has AI task",
		value: "has-ai-task",
		icon: <SparklesIcon className="size-icon-sm" />,
	},
	{
		label: "Agents allowed",
		value: "agents-allowed",
		icon: <BotIcon className="size-icon-sm" />,
	},
	{
		label: "Has external agent",
		value: "has_external_agent",
		icon: <MonitorIcon className="size-icon-sm" />,
	},
];

/**
 * Query keys the Attributes category owns, derived from its option definitions
 * so a new attribute cannot commit a chip token the parser silently drops.
 */
export const ATTRIBUTE_CHIP_KEYS: readonly string[] = ATTRIBUTE_DEFINITIONS.map(
	(attribute) => attribute.value,
);

/**
 * Boolean template attributes exposed as a single "Attributes" category. Each
 * option commits its own `key:true` chip (e.g. `deprecated:true`) rather than a
 * shared `attributes:` key, matching the backend template search filters.
 */
export const getAttributeFilterOptions = async (
	query: string,
): Promise<FilterOption[]> => {
	const normalized = query.trim().toLowerCase();

	return ATTRIBUTE_DEFINITIONS.filter(
		(attribute) =>
			normalized.length === 0 ||
			attribute.label.toLowerCase().includes(normalized) ||
			attribute.value.toLowerCase().includes(normalized),
	).map((attribute) => ({
		label: attribute.label,
		value: attribute.value,
		token: `${attribute.value}:true`,
		startIcon: attributeIcon(attribute.icon),
	}));
};

export const getOrganizationFilterOptions = async (
	query: string,
	organizations: readonly Organization[],
): Promise<FilterOption[]> => {
	const normalized = query.trim().toLowerCase();
	const mapped = organizations.map((organization) => ({
		label: organization.display_name || organization.name,
		value: organization.name,
		startIcon: (
			<Avatar
				size="md"
				fallback={organization.display_name || organization.name}
				src={organization.icon}
			/>
		),
	}));

	if (normalized.length === 0) {
		return mapped;
	}
	return mapped.filter(
		(option) =>
			option.label.toLowerCase().includes(normalized) ||
			option.value.toLowerCase().includes(normalized),
	);
};
