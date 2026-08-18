import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";

const CONTACT_SALES_LINK = "https://coder.com/contact/sales";

interface TrialActivePanelProps {
	daysRemaining: number | undefined;
	isLoading: boolean;
}

export const TrialActivePanel: FC<TrialActivePanelProps> = ({
	daysRemaining,
	isLoading,
}) => {
	const showRemaining = daysRemaining !== undefined && daysRemaining > 0;

	return (
		<div className="flex flex-col items-center gap-6 text-center max-w-md">
			<div className="flex flex-col gap-2">
				<h1 className="m-0 font-semibold text-2xl text-content-primary">
					Your Premium trial is active
				</h1>
				{isLoading ? (
					<Skeleton
						className="h-5 w-40 self-center"
						data-testid="trial-remaining-skeleton"
					/>
				) : (
					showRemaining && (
						<p className="m-0 text-sm text-content-secondary">
							{daysRemaining} {daysRemaining === 1 ? "day" : "days"} remaining
						</p>
					)
				)}
			</div>

			<p className="m-0 text-sm text-content-secondary">
				Contact our sales team to extend your trial or upgrade to Coder Premium.
			</p>

			<Button asChild>
				<a href={CONTACT_SALES_LINK} target="_blank" rel="noreferrer">
					Talk to sales
				</a>
			</Button>
		</div>
	);
};
