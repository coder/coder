import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import { hasError, isApiValidationError } from "#/api/errors";
import type { AIBridgeSession } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { AIBridgeSetupAlert } from "../AIBridgeSetupAlert";
import { ColumnsMenu } from "./ColumnsMenu";
import { ListSessionsFilter } from "./ListSessionsFilter";
import { ListSessionsRow } from "./ListSessionsRow";
import {
	chevronColumnWidth,
	type SessionColumnId,
	sessionColumns,
} from "./sessionColumns";
import { useSessionColumns } from "./useSessionColumns";

type ListSessionsPageViewProps = {
	/** Scopes the saved column settings to the signed-in user. */
	userId: string;
	isLoading: boolean;
	isFetching: boolean;
	isAISessionsEntitled: boolean;
	isAISessionsEnabled: boolean;
	sessions?: readonly AIBridgeSession[];
	sessionsQuery: PaginationResult;
	filterProps: React.ComponentProps<typeof ListSessionsFilter>;
	onSessionRowClick?: (sessionId: string) => void;
};

type HeaderInfoTooltipProps = React.PropsWithChildren<{
	label: string;
	className: string;
}>;

const HeaderInfoTooltip: React.FC<HeaderInfoTooltipProps> = ({
	label,
	className,
	children,
}) => (
	<TooltipProvider>
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={label}
					className="m-0 flex items-center border-0 bg-transparent p-0 text-inherit rounded-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<InfoIcon className="size-icon-xs" />
				</button>
			</TooltipTrigger>
			<TooltipContent
				side="top"
				align="end"
				className={cn("text-sm font-normal", className)}
			>
				{children}
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

type ColumnHeaderProps = {
	column: SessionColumnId;
	utcOffset: string;
};

const ColumnHeader: React.FC<ColumnHeaderProps> = ({ column, utcOffset }) => {
	const { label } = sessionColumns[column];
	switch (column) {
		case "threads":
			return (
				<div className="flex items-center flex-nowrap gap-1">
					{label}
					<HeaderInfoTooltip label="About threads" className="max-w-xs">
						A thread is a multi-part interaction between human and agent
						involving an initial human prompt and a subsequent agentic loop.
					</HeaderInfoTooltip>
				</div>
			);
		case "last_active":
			return (
				<div className="flex items-center flex-nowrap gap-1">
					{label}
					<HeaderInfoTooltip
						label="About last active time"
						className="max-w-60"
					>
						Times are shown in your local timezone{" "}
						<span className="whitespace-nowrap">(UTC{utcOffset}).</span>
					</HeaderInfoTooltip>
				</div>
			);
		default:
			return <span className="block truncate">{label}</span>;
	}
};

export const ListSessionsPageView: React.FC<ListSessionsPageViewProps> = ({
	userId,
	isLoading,
	isFetching,
	isAISessionsEntitled,
	isAISessionsEnabled,
	sessions,
	sessionsQuery,
	filterProps,
	onSessionRowClick,
}) => {
	const {
		settings: columnSettings,
		updateSettings: updateColumnSettings,
		tableRef,
		fittingCount,
		visibleColumns,
	} = useSessionColumns(userId);

	if (!isAISessionsEntitled) {
		return (
			<PremiumPaywallAIGovernance
				variant="sessions"
				source="aibridge_sessions"
			/>
		);
	}

	if (!isAISessionsEnabled) {
		return <AIBridgeSetupAlert />;
	}

	const utcOffset = formatDateTime(new Date(), DATE_FORMAT.UTC_OFFSET);

	return (
		<>
			<div className="mb-4 flex flex-col gap-4">
				{hasError(filterProps.error) &&
					!isApiValidationError(filterProps.error) && (
						<ErrorAlert error={filterProps.error} />
					)}
				<div className="flex items-start gap-2">
					<div className="min-w-0 flex-1">
						<ListSessionsFilter {...filterProps} />
					</div>
					<ColumnsMenu
						settings={columnSettings}
						onSettingsChange={updateColumnSettings}
						fittingCount={fittingCount}
					/>
				</div>
			</div>

			<PaginationContainer
				query={sessionsQuery}
				paginationUnitLabel={{ singular: "session", plural: "sessions" }}
			>
				<div ref={tableRef}>
					<Table
						aria-label="AI sessions"
						className="table-fixed text-sm font-normal"
					>
						<TableHeader>
							<TableRow>
								{visibleColumns.map((id) => (
									<TableHead
										key={id}
										// The last prompt takes the remaining width.
										style={
											id === "last_prompt"
												? undefined
												: { width: sessionColumns[id].width }
										}
									>
										<ColumnHeader column={id} utcOffset={utcOffset} />
									</TableHead>
								))}
								<TableHead style={{ width: chevronColumnWidth }}>
									<span className="sr-only">Open session</span>
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{isLoading || isFetching ? (
								<TableLoader />
							) : sessions?.length === 0 ? (
								<TableEmpty message="No session logs available" />
							) : (
								sessions?.map((session) => (
									<ListSessionsRow
										session={session}
										columns={visibleColumns}
										key={session.id}
										onClick={() => onSessionRowClick?.(session.id)}
									/>
								))
							)}
						</TableBody>
					</Table>
				</div>
			</PaginationContainer>
		</>
	);
};
