import { EllipsisVerticalIcon } from "lucide-react";
import { useRef, useState } from "react";
import { Link } from "react-router";
import type {
	AISpendPeriodWindow,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { queryWithTimeRange } from "#/pages/AIBridgePage/ListSessionsPage/timeRange";
import { CostDetailsDialog, type CostDetailsUser } from "./CostDetailsDialog";

type UserSpendMenuProps = {
	user: CostDetailsUser;
	/** Undefined while the user's details load, null when they have none. */
	details: OrganizationAISpendUser | null | undefined;
	detailsError: Error | null;
	period: AISpendPeriodWindow;
	organizationName: string;
	/** The sessions page needs read access to every user's sessions. */
	canViewSessions: boolean;
};

export const UserSpendMenu: React.FC<UserSpendMenuProps> = ({
	user,
	details,
	detailsError,
	period,
	organizationName,
	canViewSessions,
}) => {
	const [isCostDetailsOpen, setIsCostDetailsOpen] = useState(false);
	const menuTriggerRef = useRef<HTMLButtonElement>(null);
	const sessionsFilter = queryWithTimeRange(
		{ initiator: user.username },
		{
			start: new Date(period.period_start),
			end: new Date(period.period_end),
		},
	);

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						ref={menuTriggerRef}
						size="icon"
						variant="subtle"
						aria-label={`Open menu for ${user.name || user.username}`}
					>
						<EllipsisVerticalIcon aria-hidden />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem onClick={() => setIsCostDetailsOpen(true)}>
						Cost details
					</DropdownMenuItem>
					{canViewSessions && (
						<DropdownMenuItem asChild>
							<Link
								to={`/ai-gateway/sessions?filter=${encodeURIComponent(sessionsFilter)}`}
							>
								User AI session logs
							</Link>
						</DropdownMenuItem>
					)}
				</DropdownMenuContent>
			</DropdownMenu>
			<CostDetailsDialog
				open={isCostDetailsOpen}
				onOpenChange={setIsCostDetailsOpen}
				onCloseAutoFocus={(event) => {
					// The dialog opens from a menu item, which is gone once it closes.
					event.preventDefault();
					menuTriggerRef.current?.focus();
				}}
				user={user}
				details={details}
				detailsError={detailsError}
				period={period}
				organizationName={organizationName}
			/>
		</>
	);
};
