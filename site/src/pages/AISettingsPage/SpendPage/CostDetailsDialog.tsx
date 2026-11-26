import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import { TriangleAlertIcon } from "lucide-react";
import { useId } from "react";
import type {
	AISpendPeriodWindow,
	OrganizationAISpendModel,
	OrganizationAISpendOverviewUser,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT } from "#/utils/time";

dayjs.extend(utc);

export type CostDetailsUser = Pick<
	OrganizationAISpendOverviewUser,
	"username" | "name" | "avatar_url" | "spend_micros"
>;

type CostDetailsDialogProps = {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onCloseAutoFocus: (event: Event) => void;
	user: CostDetailsUser;
	/** Undefined while the user's details load, null when they have none. */
	details: OrganizationAISpendUser | null | undefined;
	detailsError: Error | null;
	period: AISpendPeriodWindow;
	organizationName: string;
	onOpenDetailedReport?: () => void;
};

export const CostDetailsDialog: React.FC<CostDetailsDialogProps> = ({
	open,
	onOpenChange,
	onCloseAutoFocus,
	user,
	details,
	detailsError,
	period,
	organizationName,
	onOpenDetailedReport,
}) => {
	const displayName = user.name || user.username;
	const startDate = dayjs
		.utc(period.period_start)
		.format(DATE_FORMAT.MEDIUM_DATE);
	// The response's period end is exclusive, so subtract a day to show the
	// last day included in the spend.
	const endDate = dayjs
		.utc(period.period_end)
		.subtract(1, "day")
		.format(DATE_FORMAT.MEDIUM_DATE);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-xl" onCloseAutoFocus={onCloseAutoFocus}>
				<DialogHeader className="flex-row items-start justify-between gap-4 space-y-0">
					<DialogTitle>Cost details</DialogTitle>
					<AvatarData
						title={displayName}
						subtitle={`@${user.username}`}
						src={user.avatar_url}
					/>
				</DialogHeader>
				<DialogDescription className="font-normal">
					{displayName} has spent {formatCostMicros(user.spend_micros)} from{" "}
					{startDate} to {endDate} in the {organizationName} organization.
				</DialogDescription>
				{detailsError !== null && <ErrorAlert error={detailsError} />}
				{details === undefined && detailsError === null && (
					<div className="flex flex-col gap-6 rounded-lg border border-border p-5">
						{Array.from({ length: 2 }, (_, section) => (
							<div key={section} className="flex flex-col gap-3">
								<Skeleton className="h-4 w-24" />
								{Array.from({ length: 4 }, (_, row) => (
									<Skeleton key={row} className="h-4" />
								))}
							</div>
						))}
					</div>
				)}
				{details === null && (
					<p className="m-0 rounded-lg border border-border p-5 text-sm text-content-secondary">
						Cost details aren't available for this user.
					</p>
				)}
				{details && (
					<ScrollArea
						className="rounded-lg border border-border"
						viewportClassName="max-h-96 [&>div]:block!"
						viewportTabIndex={0}
						viewportAriaLabel="Cost breakdown"
					>
						<div className="flex flex-col gap-6 p-5">
							<SpendSection
								title="Providers"
								items={details.provider_spend.map((provider) => ({
									...provider,
									icon: (
										<ProviderIcon
											provider={provider.type ?? ""}
											icon={provider.icon}
										/>
									),
								}))}
							/>
							<SpendSection
								title="Models"
								items={details.model_spend.map((model) => ({
									...model,
									icon: (
										<AIBridgeModelIcon
											model={model.name}
											className="size-icon-sm"
										/>
									),
								}))}
							/>
							<SpendSection
								title="Clients"
								items={details.client_spend.map((client) => ({
									...client,
									icon: (
										<AIBridgeClientIcon
											client={client.name}
											className="size-icon-sm"
										/>
									),
								}))}
							/>
						</div>
					</ScrollArea>
				)}
				<DialogFooter className="sm:justify-between">
					<Button variant="outline" onClick={() => onOpenChange(false)}>
						Close
					</Button>
					{onOpenDetailedReport && (
						<Button onClick={onOpenDetailedReport}>Run detailed report</Button>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

type SpendItem = OrganizationAISpendModel & { icon?: React.ReactNode };

type SpendSectionProps = {
	title: string;
	items: readonly SpendItem[];
};

const SpendSection: React.FC<SpendSectionProps> = ({ title, items }) => {
	const titleId = useId();
	return (
		<section aria-labelledby={titleId} className="flex flex-col gap-3">
			<h3
				id={titleId}
				className="m-0 text-sm font-normal text-content-secondary"
			>
				{title}
			</h3>
			<ul className="m-0 flex list-none flex-col gap-3 p-0">
				{items.map((item) => (
					<li key={item.name} className="flex items-center gap-2 text-sm">
						{item.icon}
						<span className="min-w-0 flex-1 truncate font-mono">
							{item.name}
						</span>
						<SpendItemCost item={item} />
					</li>
				))}
			</ul>
		</section>
	);
};

type SpendItemCostProps = {
	item: SpendItem;
};

const SpendItemCost: React.FC<SpendItemCostProps> = ({ item }) => {
	if (item.has_unpriced_usage && item.cost_micros === 0) {
		return (
			<Badge variant="warning">
				<TriangleAlertIcon aria-hidden />
				No pricing
			</Badge>
		);
	}
	return (
		<>
			{item.has_unpriced_usage && (
				<Badge variant="warning">
					<TriangleAlertIcon aria-hidden />
					<span className="sr-only">Some usage has no pricing</span>
				</Badge>
			)}
			<span className="tabular-nums">{formatCostMicros(item.cost_micros)}</span>
		</>
	);
};
