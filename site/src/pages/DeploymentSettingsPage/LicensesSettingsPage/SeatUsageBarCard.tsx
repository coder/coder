import type { FC } from "react";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { cn } from "#/utils/cn";

type SeatUsageBarCardProps = {
	title: string;
	actual: number | undefined;
	limit: number | undefined;
	allowUnlimited?: boolean;
};

export const SeatUsageBarCard: FC<SeatUsageBarCardProps> = ({
	title,
	actual,
	limit,
	allowUnlimited = false,
}) => {
	const isUnlimited = allowUnlimited && limit === undefined;

	if (!isUnlimited && (limit === undefined || limit < 0)) {
		return (
			<section className="border border-solid rounded">
				<div className="p-4">
					<ErrorAlert error="Invalid license usage limits" />
				</div>
			</section>
		);
	}

	const meteredLimit = limit ?? 0;
	const activeNum = actual ?? 0;
	const isExceeded =
		!isUnlimited && actual !== undefined && actual > meteredLimit;
	const usagePercentage = isUnlimited
		? 100
		: meteredLimit > 0
			? Math.min((activeNum / meteredLimit) * 100, 100)
			: 0;

	const activeLabel =
		actual === undefined ? "—" : activeNum.toLocaleString("en-US");
	const limitLabel = isUnlimited
		? "Unlimited"
		: meteredLimit.toLocaleString("en-US");

	return (
		<section className={cn("border border-solid rounded")}>
			<div className="p-4">
				<div className="flex flex-col gap-2">
					<h3 className="text-md m-0 font-medium">{title}</h3>

					<div
						className="relative h-5 w-full overflow-hidden rounded bg-surface-secondary"
						aria-hidden="true"
					>
						<div
							className={cn(
								"h-full rounded-l transition-[width] duration-300",
								isExceeded ? "bg-highlight-red" : "bg-highlight-green",
							)}
							style={{ width: `${usagePercentage}%` }}
						/>
					</div>

					<div className="flex items-start justify-between text-sm font-medium whitespace-nowrap">
						<p className="m-0 text-content-primary">
							<span className="text-content-secondary">Active: </span>
							<span
								className={cn({
									"text-content-destructive": isExceeded,
								})}
							>
								{activeLabel}
							</span>
						</p>
						<p className="m-0 text-content-secondary">
							Limit: <span className="text-content-primary">{limitLabel}</span>
						</p>
					</div>
				</div>
			</div>
		</section>
	);
};
