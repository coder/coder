import cronstrue from "cronstrue";
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
import { useRef, useState } from "react";
import { Link } from "react-router";
import type {
	ChatAutomation,
	ChatAutomationWebhookAuth,
	Organization,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
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
import {
	StatusIndicator,
	StatusIndicatorDot,
} from "#/components/StatusIndicator/StatusIndicator";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { RetryButton } from "../components/RetryButton";
import { SectionHeader } from "../components/SectionHeader";

const webhookAuthLabels: Record<ChatAutomationWebhookAuth, string> = {
	bearer: "Bearer token",
	github: "GitHub",
	standard_webhooks: "Standard Webhooks",
};

const automationOwnerName = (owner: ChatAutomation["owner"]) =>
	owner?.name || owner?.username || "Unknown";

const automationStatus = (automation: ChatAutomation) => {
	if (!automation.enabled) {
		return "disabled";
	}
	return automation.paused_reasons.length > 0 ? "paused" : "enabled";
};

const scheduleLabel = (automation: ChatAutomation) =>
	automation.schedule_cron
		? cronstrue.toString(automation.schedule_cron, {
				throwExceptionOnParseError: false,
			})
		: "Schedule unavailable";

const automationStatuses = {
	enabled: { label: "Enabled", variant: "success" },
	disabled: { label: "Disabled", variant: "inactive" },
	paused: { label: "Paused", variant: "warning" },
} as const;

const statusOptions: readonly FilterOption[] = Object.entries(
	automationStatuses,
).map(([value, { label, variant }]) => ({
	label,
	value,
	startIcon: <StatusIndicatorDot variant={variant} size="md" />,
}));

const triggerOptions: readonly FilterOption[] = [
	{
		label: "Webhook",
		value: "webhook",
		startIcon: <WebhookIcon />,
	},
	{
		label: "Schedule",
		value: "schedule",
		startIcon: <CalendarClockIcon />,
	},
];

type AgentAutomationsPageViewProps = {
	automations: readonly ChatAutomation[];
	organizations: readonly Organization[];
	isLoading: boolean;
	loadErrors: readonly {
		organization: Organization;
		error: unknown;
		isRetrying: boolean;
	}[];
	onRetry: (organizationId: string) => void;
	onMenuTrigger: React.ReactEventHandler<HTMLButtonElement>;
	onDeleteAutomation: (automation: ChatAutomation) => void;
};

/** Renders a filterable automation list alongside per-organization load errors. */
export const AgentAutomationsPageView: React.FC<
	AgentAutomationsPageViewProps
> = ({
	automations,
	organizations,
	isLoading,
	loadErrors,
	onRetry,
	onMenuTrigger,
	onDeleteAutomation,
}) => {
	const [filterQuery, setFilterQuery] = useState("");
	const ownersByUsername = new Map(
		automations.flatMap((automation) =>
			automation.owner
				? [[automation.owner.username, automation.owner] as const]
				: [],
		),
	);
	const ownerOptions: readonly FilterOption[] = Array.from(
		ownersByUsername.values(),
		(owner) => {
			const ownerName = automationOwnerName(owner);
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
	const normalizedQuery = extractFreeText(filterQuery, [
		"owner",
		"trigger",
		"status",
	]).toLocaleLowerCase("en-US");
	const filteredAutomations = automations.filter((automation) => {
		const ownerName = automationOwnerName(automation.owner);
		const triggerText =
			automation.kind === "schedule"
				? `${scheduleLabel(automation)} ${automation.schedule_time_zone ?? ""}`
				: `Webhook ${automation.webhook_auth ? webhookAuthLabels[automation.webhook_auth] : ""}`;
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
			automationStatus(automation) === filterValues.status;
		const matchesTrigger =
			filterValues.trigger === undefined ||
			automation.kind === filterValues.trigger;
		const matchesOwner =
			filterValues.owner === undefined ||
			automation.owner?.username === filterValues.owner;

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
						<Link to="/agents/automations/new">
							<PlusIcon />
							Create automation
						</Link>
					</Button>
				}
			/>
			<div className="flex flex-col gap-4">
				<p className="m-0 text-sm text-content-secondary">
					This preview supports creating, viewing, and deleting automation
					configurations. Editing, execution, and changing an existing
					automation’s enabled state are not available yet.
				</p>
				{loadErrors.map(({ organization, error, isRetrying }) => (
					<div key={organization.id} className="flex flex-col gap-2">
						<p className="m-0 text-sm text-content-secondary">
							Could not load automations for{" "}
							{organization.display_name || organization.name}.
						</p>
						<ErrorAlert
							error={error}
							showDebugDetail={false}
							actions={
								<RetryButton
									size="sm"
									variant="outline"
									aria-label={`Retry loading automations for ${organization.display_name || organization.name}`}
									isRetrying={isRetrying}
									onRetry={() => onRetry(organization.id)}
								/>
							}
						/>
					</div>
				))}
				<FilterCombobox
					value={filterQuery}
					onChange={setFilterQuery}
					categories={filterCategories}
					placeholder="Search and filter automations…"
					className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
				/>
				{(automations.length > 0 || isLoading || loadErrors.length === 0) && (
					<Table
						aria-label="Automations"
						wrapperProps={{
							tabIndex: 0,
							role: "region",
							"aria-label": "Automations table",
						}}
					>
						<TableHeader>
							<TableRow>
								<TableHead>Status</TableHead>
								<TableHead>Name</TableHead>
								<TableHead>Owner</TableHead>
								{organizations.length > 1 && (
									<TableHead>Organization</TableHead>
								)}
								<TableHead>Trigger</TableHead>
								<TableHead>
									<span className="sr-only">Actions</span>
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody size="lg">
							{filteredAutomations.length === 0 &&
								!isLoading &&
								(hasActiveFilters || loadErrors.length === 0) && (
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
								)}
							{filteredAutomations.map((automation) => (
								<AutomationRow
									key={automation.id}
									automation={automation}
									organization={organizations.find(
										(org) => org.id === automation.organization_id,
									)}
									showOrganization={organizations.length > 1}
									onMenuTrigger={onMenuTrigger}
									onDeleteAutomation={onDeleteAutomation}
								/>
							))}
							{isLoading && <TableLoader />}
						</TableBody>
					</Table>
				)}
			</div>
		</div>
	);
};

type AutomationRowProps = {
	automation: ChatAutomation;
	organization: Organization | undefined;
	showOrganization: boolean;
	onMenuTrigger: React.ReactEventHandler<HTMLButtonElement>;
	onDeleteAutomation: (automation: ChatAutomation) => void;
};

const AutomationRow: React.FC<AutomationRowProps> = ({
	automation,
	organization,
	showOrganization,
	onMenuTrigger,
	onDeleteAutomation,
}) => {
	const deleteSelectedRef = useRef(false);
	const status = automationStatuses[automationStatus(automation)];
	const ownerName = automationOwnerName(automation.owner);

	return (
		<TableRow>
			<TableCell className="w-0">
				<StatusIndicator
					variant={status.variant}
					className="text-content-primary"
				>
					<StatusIndicatorDot />
					{status.label}
				</StatusIndicator>
			</TableCell>
			<TableCell className="w-full min-w-48 max-w-0">
				<div className="truncate text-content-primary" title={automation.name}>
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
						fallback={ownerName}
						src={automation.owner?.avatar_url}
					/>
					<span className="truncate text-content-primary">{ownerName}</span>
				</div>
			</TableCell>
			{showOrganization && (
				<TableCell className="max-w-40 truncate">
					{organization?.display_name || organization?.name || "Unknown"}
				</TableCell>
			)}
			<TableCell className="min-w-48 max-w-64">
				<div className="flex items-start gap-3">
					{automation.kind === "schedule" ? (
						<CalendarClockIcon className="mt-0.5 size-4 shrink-0" />
					) : (
						<WebhookIcon className="mt-0.5 size-4 shrink-0" />
					)}
					<div className="min-w-0">
						<div className="text-content-primary">
							{automation.kind === "schedule"
								? scheduleLabel(automation)
								: "Webhook"}
						</div>
						<div>
							{automation.kind === "schedule"
								? automation.schedule_time_zone || "Unknown time zone"
								: automation.webhook_auth
									? webhookAuthLabels[automation.webhook_auth]
									: "Authentication unavailable"}
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
							onPointerDown={onMenuTrigger}
							onKeyDown={onMenuTrigger}
						>
							<EllipsisVerticalIcon aria-hidden="true" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="end"
						onCloseAutoFocus={(event) => {
							if (deleteSelectedRef.current) {
								deleteSelectedRef.current = false;
								event.preventDefault();
								// The menu must release focus before the dialog takes it.
								onDeleteAutomation(automation);
							}
						}}
					>
						<DropdownMenuItem disabled>
							<PlayIcon />
							Run now
						</DropdownMenuItem>
						{automation.target_mode === "existing_chat" &&
							automation.target_chat && (
								<DropdownMenuItem asChild>
									<Link
										to={`/agents/${encodeURIComponent(automation.target_chat.id)}`}
									>
										<MessagesSquareIcon />
										View chat
									</Link>
								</DropdownMenuItem>
							)}
						<DropdownMenuItem disabled>
							<PencilIcon />
							Edit
						</DropdownMenuItem>
						<DropdownMenuSeparator />
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onSelect={() => {
								deleteSelectedRef.current = true;
							}}
						>
							<TrashIcon />
							Delete&hellip;
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};
