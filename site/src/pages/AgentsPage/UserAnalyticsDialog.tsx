import { chatCostSummary } from "api/queries/chats";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { useAuthContext } from "contexts/auth/AuthProvider";
import dayjs from "dayjs";
import { BarChart3Icon, XIcon } from "lucide-react";
import { type FC, useMemo } from "react";
import { useQuery } from "react-query";
import { ChatCostSummaryView } from "./ChatCostSummaryView";
import { SectionHeader } from "./SectionHeader";

const createDateRange = () => {
	const end = dayjs();
	const start = end.subtract(30, "day");
	return {
		startDate: start.toISOString(),
		endDate: end.toISOString(),
		rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
	};
};

interface UserAnalyticsDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
}

export const UserAnalyticsDialog: FC<UserAnalyticsDialogProps> = ({
	open,
	onOpenChange,
}) => {
	const { user } = useAuthContext();
	const dateRange = useMemo(createDateRange, []);

	const summaryQuery = useQuery({
		...chatCostSummary(user?.id ?? "me", {
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
		enabled: open && Boolean(user?.id),
	});

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-4xl overflow-hidden p-0">
				<DialogHeader className="sr-only">
					<DialogTitle>Analytics</DialogTitle>
					<DialogDescription>
						Review your personal chat usage for the last 30 days.
					</DialogDescription>
				</DialogHeader>
				<div className="flex max-h-[min(88dvh,720px)] min-h-0 flex-col overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
					<div className="mb-6 flex items-start justify-between gap-4">
						<SectionHeader
							label="Analytics"
							description="Review your personal chat usage and cost breakdowns."
							action={
								<div className="flex items-center gap-2 text-xs text-content-secondary">
									<BarChart3Icon className="h-4 w-4" />
									<span>{dateRange.rangeLabel}</span>
								</div>
							}
						/>
						<DialogClose asChild>
							<Button
								variant="subtle"
								size="icon-lg"
								className="shrink-0 border-none bg-transparent shadow-none hover:bg-surface-tertiary/50"
							>
								<XIcon className="text-content-secondary" />
								<span className="sr-only">Close</span>
							</Button>
						</DialogClose>
					</div>

					<ChatCostSummaryView
						summary={summaryQuery.data}
						isLoading={summaryQuery.isLoading}
						error={summaryQuery.error}
						onRetry={() => void summaryQuery.refetch()}
						loadingLabel="Loading analytics"
						emptyMessage="No usage data for you in this period."
					/>
				</div>
			</DialogContent>
		</Dialog>
	);
};
