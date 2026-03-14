import type { ChatUsageLimitPeriod } from "api/typesGenerated";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Switch } from "components/Switch/Switch";
import { TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { SectionHeader } from "../SectionHeader";

interface DefaultLimitSectionProps {
	enabled: boolean;
	onEnabledChange: (enabled: boolean) => void;
	period: ChatUsageLimitPeriod;
	onPeriodChange: (period: ChatUsageLimitPeriod) => void;
	amountDollars: string;
	onAmountDollarsChange: (amount: string) => void;
	unpricedModelCount: number;
	adminBadge: ReactNode;
	panelClassName: string;
}

export const DefaultLimitSection: FC<DefaultLimitSectionProps> = ({
	enabled,
	onEnabledChange,
	period,
	onPeriodChange,
	amountDollars,
	onAmountDollarsChange,
	unpricedModelCount,
	adminBadge,
	panelClassName,
}) => (
	<>
		<SectionHeader
			label="Default Spend Limit"
			description="Set a deployment-wide spend cap that applies to all users by default."
			badge={adminBadge}
		/>

		<div className={panelClassName}>
			<div className="flex items-center justify-between gap-4">
				<div>
					<p className="m-0 text-sm font-medium text-content-primary">
						Enable spend limit
					</p>
					<p className="m-0 text-xs text-content-secondary">
						When disabled, users have unlimited spending.
					</p>
				</div>
				<Switch checked={enabled} onCheckedChange={onEnabledChange} />
			</div>

			{enabled && (
				<div className="flex flex-col gap-3 md:flex-row md:items-end">
					<div className="flex-1 space-y-1">
						<Label htmlFor="chat-limit-period">Period</Label>
						<Select
							value={period}
							onValueChange={(value) =>
								onPeriodChange(value as ChatUsageLimitPeriod)
							}
						>
							<SelectTrigger
								id="chat-limit-period"
								className="h-9 min-w-0 text-[13px]"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="day">Day</SelectItem>
								<SelectItem value="week">Week</SelectItem>
								<SelectItem value="month">Month</SelectItem>
							</SelectContent>
						</Select>
					</div>
					<div className="flex-1 space-y-1">
						<Label htmlFor="chat-limit-amount">Amount ($)</Label>
						<Input
							id="chat-limit-amount"
							type="number"
							step="0.01"
							min="0"
							className="h-9 min-w-0 text-[13px]"
							value={amountDollars}
							onChange={(event) => onAmountDollarsChange(event.target.value)}
							placeholder="0.00"
						/>
					</div>
				</div>
			)}
		</div>

		{enabled && unpricedModelCount > 0 && (
			<div className="flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary">
				<TriangleAlertIcon className="h-5 w-5 shrink-0 text-content-warning" />
				<div>
					{unpricedModelCount === 1
						? "1 enabled model does not have pricing configured."
						: `${unpricedModelCount} enabled models do not have pricing configured.`}{" "}
					Usage of unpriced models cannot be tracked against the spend limit.
				</div>
			</div>
		)}
	</>
);
