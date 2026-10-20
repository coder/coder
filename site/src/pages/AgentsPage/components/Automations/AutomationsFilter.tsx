import {
	CalendarClockIcon,
	CircleDotIcon,
	UserIcon,
	WebhookIcon,
	ZapIcon,
} from "lucide-react";
import { useQueryClient } from "react-query";
import type { ChatAutomation } from "#/api/typesGenerated";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import {
	extractFreeText,
	filterOptionsByText,
} from "#/components/Filter/FilterCombobox/filterQuery";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { parseFilterQuery } from "#/components/Filter/filterQuery";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getSelfUserFilterOptions,
	getUserFilterOptions,
} from "#/pages/WorkspacesPage/filter/categoryOptions";
import {
	type AutomationStatusValue,
	getAutomationStatus,
} from "./AutomationStatus";

/** The list opens on the viewer's own automations, like the Workspaces page. */
export const defaultAutomationsFilter = "owner:me";

const filterKeys = ["owner", "trigger", "status"];

const triggerOptions: FilterOption[] = [
	{ label: "Webhook", value: "webhook", startIcon: <WebhookIcon /> },
	{ label: "Schedule", value: "schedule", startIcon: <CalendarClockIcon /> },
];

const statusDot = (variant: "success" | "inactive" | "warning") => (
	<StatusIndicatorDot variant={variant} size="md" />
);

const statusOptions: FilterOption[] = [
	{ label: "Enabled", value: "enabled", startIcon: statusDot("success") },
	{ label: "Disabled", value: "disabled", startIcon: statusDot("inactive") },
	{ label: "Paused", value: "paused", startIcon: statusDot("warning") },
	{ label: "Used", value: "used", startIcon: statusDot("inactive") },
] satisfies Array<FilterOption & { value: AutomationStatusValue }>;

/**
 * Keeps the automations that match the trigger, status and name parts of a
 * filter query. The owner part goes to the API as its owner parameter.
 */
export const filterAutomations = (
	automations: readonly ChatAutomation[],
	query: string,
): readonly ChatAutomation[] => {
	// The list API is not paginated and filters only by owner, so trigger,
	// status and name are filtered here, over the full list it returned.
	const values = parseFilterQuery(query);
	const trigger = values.trigger?.toLowerCase();
	const status = values.status?.toLowerCase();
	const name = extractFreeText(query, filterKeys).toLowerCase();
	return automations.filter(
		(automation) =>
			(!trigger || automation.kind === trigger) &&
			(!status || getAutomationStatus(automation) === status) &&
			(!name || automation.name.toLowerCase().includes(name)),
	);
};

type AutomationsFilterProps = {
	query: string;
	onChange: (query: string) => void;
	/** Offers every user as an owner, for viewers who can read their automations. */
	canFilterAnyOwner: boolean;
};

export const AutomationsFilter: React.FC<AutomationsFilterProps> = ({
	query,
	onChange,
	canFilterAnyOwner,
}) => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();
	const categories: FilterCategory[] = [
		{
			key: "owner",
			label: "Owner",
			icon: <UserIcon />,
			getOptions: canFilterAnyOwner
				? (text) => getUserFilterOptions(text, me, queryClient)
				: (text) => getSelfUserFilterOptions(text, me),
		},
		{
			key: "trigger",
			label: "Trigger",
			icon: <ZapIcon />,
			inlineOptions: true,
			inlineOptionsIcons: true,
			getOptions: async (text) => [
				...filterOptionsByText(triggerOptions, text),
			],
		},
		{
			key: "status",
			label: "Status",
			icon: <CircleDotIcon />,
			inlineOptions: true,
			inlineOptionsIcons: true,
			getOptions: async (text) => [...filterOptionsByText(statusOptions, text)],
		},
	];
	return (
		<FilterCombobox
			value={query}
			onChange={onChange}
			categories={categories}
			placeholder="Search and filter automations…"
			// Full width on mobile. From `sm` up it starts at a compact width
			// and widens to fit chips before wrapping.
			className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
		/>
	);
};
