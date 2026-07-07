import { TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type { ChatUsageLimitPeriod } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";

interface DefaultLimitSectionProps {
	enabled: boolean;
	onEnabledChange: (enabled: boolean) => void;
	period: ChatUsageLimitPeriod;
	onPeriodChange: (period: ChatUsageLimitPeriod) => void;
	amountDollars: string;
	onAmountDollarsChange: (amount: string) => void;
	unpricedModelCount: number;
	isSaving?: boolean;
	isSavedVisible?: boolean;
	saveDisabled?: boolean;
	onSave?: () => void;
	saveStatus?: ReactNode;
}

export const DefaultLimitSection: FC<DefaultLimitSectionProps> = ({
	enabled,
	onEnabledChange,
	period,
	onPeriodChange,
	amountDollars,
	onAmountDollarsChange,
	unpricedModelCount,
	isSaving = false,
	isSavedVisible = false,
	saveDisabled = false,
	onSave,
	saveStatus,
}) => {
	return (
		<section className="space-y-4">
			<div className="flex items-start gap-3">
				<Switch
					checked={enabled}
					onCheckedChange={onEnabledChange}
					aria-label="Spend limit"
					className="mt-0.5"
				/>
				<div className="flex max-w-[980px] flex-1 flex-col">
					<h3 className="m-0 text-sm font-normal leading-6 text-content-primary">
						Spend limit
					</h3>
					<p className="mt-1 mb-0 text-sm font-normal leading-6 text-content-secondary">
						Set a deployment-wide spend cap that applies to all users by
						default. When disabled, users have unlimited spending.
					</p>
					<div className="mt-4 flex flex-wrap items-start gap-3">
						<InputGroup className="w-32">
							<InputGroupAddon>$</InputGroupAddon>
							<InputGroupInput
								type="number"
								step="0.01"
								min="0"
								className="text-sm"
								value={amountDollars}
								onChange={(event) => onAmountDollarsChange(event.target.value)}
								placeholder="0.00"
								aria-label="Spend limit amount in dollars"
								disabled={!enabled}
							/>
						</InputGroup>
						<Select
							value={period}
							onValueChange={(value) =>
								onPeriodChange(value as ChatUsageLimitPeriod)
							}
							disabled={!enabled}
						>
							<SelectTrigger
								className="h-10 w-32 text-sm"
								aria-label="Spend limit period"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="day">Day</SelectItem>
								<SelectItem value="week">Week</SelectItem>
								<SelectItem value="month">Month</SelectItem>
							</SelectContent>
						</Select>
						<div className="flex min-h-10 items-center">
							{(onSave || isSavedVisible || isSaving) &&
								(isSavedVisible ? (
									saveStatus
								) : (
									<Button
										size="lg"
										type="button"
										disabled={saveDisabled}
										onClick={onSave}
										className="h-10 min-w-[88px]"
									>
										{isSaving && <Spinner loading className="size-4" />}
										Save
									</Button>
								))}
						</div>
					</div>
					{saveStatus && !isSavedVisible && (
						<div className="text-xs text-content-destructive">{saveStatus}</div>
					)}
				</div>
			</div>

			{enabled && unpricedModelCount > 0 && (
				<div className="ml-12 flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary">
					<TriangleAlertIcon className="size-5 shrink-0 text-content-warning" />
					<div>
						{unpricedModelCount === 1
							? "1 enabled model does not have pricing configured."
							: `${unpricedModelCount} enabled models do not have pricing configured.`}{" "}
						Usage of unpriced models cannot be tracked against the spend limit.
					</div>
				</div>
			)}
		</section>
	);
};
