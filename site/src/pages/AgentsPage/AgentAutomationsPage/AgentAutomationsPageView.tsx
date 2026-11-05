import {
	CalendarClockIcon,
	CircleDotIcon,
	EllipsisVerticalIcon,
	MessagesSquareIcon,
	PencilIcon,
	PlayIcon,
	PlusIcon,
	TrashIcon,
	UserIcon,
	WebhookIcon,
	ZapIcon,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import type { MinimalUser } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { ExperimentalBadge } from "#/components/Badge/PresetBadges";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
import { Switch } from "#/components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { SectionHeader } from "../components/SectionHeader";

type AutomationTrigger =
	| {
			type: "timetable";
			label: string;
			timezone: string;
	  }
	| {
			type: "webhook";
			usage: string;
	  };

const statusOptions: readonly FilterOption[] = [
	{
		label: "Enabled",
		value: "enabled",
		startIcon: <StatusIndicatorDot variant="success" size="md" />,
	},
	{
		label: "Disabled",
		value: "disabled",
		startIcon: <StatusIndicatorDot variant="inactive" size="md" />,
	},
	{
		label: "Paused",
		value: "paused",
		startIcon: <StatusIndicatorDot variant="warning" size="md" />,
	},
	{
		label: "Used",
		value: "used",
		startIcon: <StatusIndicatorDot variant="inactive" size="md" />,
	},
];

const triggerOptions: readonly FilterOption[] = [
	{
		label: "Webhook",
		value: "webhook",
		startIcon: <WebhookIcon />,
	},
	{
		label: "Timetable",
		value: "timetable",
		startIcon: <CalendarClockIcon />,
	},
];

const filterKeys = ["owner", "trigger", "status"];

/** An automation started by either a timetable or webhook. */
export type Automation = {
	id: string;
	name: string;
	prompt: string;
	owner: Pick<MinimalUser, "username" | "name" | "avatar_url">;
	trigger: AutomationTrigger;
	enabled: boolean;
};

type AgentAutomationsPageViewProps = {
	automations: readonly Automation[];
	onEnabledChange: (id: string, enabled: boolean) => void;
	onEditAutomation: (automation: Automation) => void;
	onRunAutomation: (automation: Automation) => void;
	onViewChats: (automation: Automation) => void;
	onDeleteAutomation: (automation: Automation) => void;
};

/**
 * Displays automation triggers. The caller owns each enabled state and
 * handles row actions through the corresponding callbacks.
 */
export const AgentAutomationsPageView: React.FC<
	AgentAutomationsPageViewProps
> = ({
	automations,
	onEnabledChange,
	onEditAutomation,
	onRunAutomation,
	onViewChats,
	onDeleteAutomation,
}) => {
	const [filterQuery, setFilterQuery] = useState("");
	const ownersByUsername = new Map(
		automations.map((automation) => [
			automation.owner.username,
			automation.owner,
		]),
	);
	const ownerOptions: readonly FilterOption[] = Array.from(
		ownersByUsername.values(),
		(owner) => {
			const ownerName = owner.name || owner.username;
			return {
				label: ownerName,
				value: owner.username,
				startIcon: (
					<Avatar size="sm" fallback={ownerName} src={owner.avatar_url} />
				),
			};
		},
	);
	const filterCategories: readonly FilterCategory[] = [
		{
			key: "owner",
			label: "Owner",
			icon: <UserIcon />,
			getOptions: async (query) => [
				...filterOptionsByText(ownerOptions, query),
			],
		},
		{
			key: "trigger",
			label: "Trigger",
			icon: <ZapIcon />,
			inlineOptions: true,
			inlineOptionsIcons: true,
			getOptions: async (query) => [
				...filterOptionsByText(triggerOptions, query),
			],
		},
		{
			key: "status",
			label: "Status",
			icon: <CircleDotIcon />,
			inlineOptions: true,
			inlineOptionsIcons: true,
			getOptions: async (query) => [
				...filterOptionsByText(statusOptions, query),
			],
		},
	];
	const filterValues = parseFilterQuery(filterQuery);
	const normalizedQuery = extractFreeText(
		filterQuery,
		filterKeys,
	).toLocaleLowerCase("en-US");
	const filteredAutomations = automations.filter((automation) => {
		const ownerName = automation.owner.name || automation.owner.username;
		const triggerText =
			automation.trigger.type === "timetable"
				? `${automation.trigger.label} ${automation.trigger.timezone}`
				: `Webhook ${automation.trigger.usage}`;
		const matchesSearch = [
			automation.name,
			automation.prompt,
			ownerName,
			triggerText,
		].some((value) =>
			value.toLocaleLowerCase("en-US").includes(normalizedQuery),
		);
		const matchesStatus =
			filterValues.status === undefined ||
			(filterValues.status === "enabled" && automation.enabled) ||
			(filterValues.status === "disabled" && !automation.enabled);
		const matchesTrigger =
			filterValues.trigger === undefined ||
			automation.trigger.type === filterValues.trigger;
		const matchesOwner =
			filterValues.owner === undefined ||
			automation.owner.username === filterValues.owner;

		return matchesSearch && matchesStatus && matchesTrigger && matchesOwner;
	});
	const hasActiveFilters = filterQuery.trim() !== "";

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Automations"
				description="Manage automations for your agents."
				badge={<ExperimentalBadge />}
				action={
					<Button asChild>
						<Link to="#">
							<PlusIcon />
							Create automation
						</Link>
					</Button>
				}
			/>
			<div className="flex flex-col gap-4">
				<FilterCombobox
					value={filterQuery}
					onChange={setFilterQuery}
					categories={filterCategories}
					placeholder="Search and filter automations…"
					className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
				/>
				<Table aria-label="Automations">
					<TableHeader>
						<TableRow>
							<TableHead>
								<span className="sr-only">Enabled</span>
							</TableHead>
							<TableHead>Name</TableHead>
							<TableHead>Owner</TableHead>
							<TableHead>Trigger</TableHead>
							<TableHead>
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody size="lg">
						{filteredAutomations.length === 0 ? (
							<TableEmpty
								message={
									hasActiveFilters
										? "No matching automations"
										: "No automations yet"
								}
								description={
									hasActiveFilters
										? "Try adjusting your search or filters."
										: "Your automations will appear here."
								}
							/>
						) : (
							filteredAutomations.map((automation) => (
								<TableRow key={automation.id}>
									<TableCell className="w-0">
										<Switch
											aria-label={`Enable ${automation.name}`}
											checked={automation.enabled}
											onCheckedChange={(enabled) =>
												onEnabledChange(automation.id, enabled)
											}
										/>
									</TableCell>
									<TableCell className="w-full min-w-48 max-w-0">
										<div className="text-content-primary">
											{automation.name}
										</div>
										<p className="m-0 truncate" title={automation.prompt}>
											{automation.prompt}
										</p>
									</TableCell>
									<TableCell className="w-40 max-w-40">
										<div className="flex items-center gap-2">
											<Avatar
												size="sm"
												fallback={
													automation.owner.name || automation.owner.username
												}
												src={automation.owner.avatar_url}
											/>
											<span className="truncate text-content-primary">
												{automation.owner.name || automation.owner.username}
											</span>
										</div>
									</TableCell>
									<TableCell className="min-w-48">
										<div className="flex items-start gap-3">
											{automation.trigger.type === "timetable" ? (
												<CalendarClockIcon className="mt-0.5 size-4 shrink-0" />
											) : (
												<WebhookIcon className="mt-0.5 size-4 shrink-0" />
											)}
											<div className="min-w-0">
												<div className="text-content-primary">
													{automation.trigger.type === "timetable"
														? automation.trigger.label
														: "Webhook"}
												</div>
												<div>
													{automation.trigger.type === "timetable"
														? automation.trigger.timezone
														: automation.trigger.usage}
												</div>
											</div>
										</div>
									</TableCell>
									<TableCell className="w-0">
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													aria-label={`More actions for ${automation.name}`}
												>
													<EllipsisVerticalIcon aria-hidden="true" />
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem
													onClick={() => onRunAutomation(automation)}
												>
													<PlayIcon />
													Run now
												</DropdownMenuItem>
												<DropdownMenuItem
													onClick={() => onViewChats(automation)}
												>
													<MessagesSquareIcon />
													View chats
												</DropdownMenuItem>
												<DropdownMenuItem
													onClick={() => onEditAutomation(automation)}
												>
													<PencilIcon />
													Edit
												</DropdownMenuItem>
												<DropdownMenuSeparator />
												<DropdownMenuItem
													className="text-content-destructive focus:text-content-destructive"
													onClick={() => onDeleteAutomation(automation)}
												>
													<TrashIcon />
													Delete&hellip;
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>
		</div>
	);
};
