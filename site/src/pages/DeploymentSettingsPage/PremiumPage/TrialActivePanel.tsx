import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { CONTACT_SALES_LINK } from "#/modules/licenses/trialLicense";

interface TrialActivePanelProps {
	daysRemaining: number | undefined;
}

export const TrialActivePanel: FC<TrialActivePanelProps> = ({
	daysRemaining,
}) => {
	const showRemaining = daysRemaining !== undefined && daysRemaining > 0;

	return (
		<div className="flex flex-col items-center gap-6 text-center max-w-md">
			{showRemaining ? (
				<div className="flex flex-col gap-2">
					<h1 className="m-0 font-semibold text-3xl text-content-primary">
						{daysRemaining} {daysRemaining === 1 ? "day" : "days"} remaining
					</h1>
				</div>
			) : null}

			<p className="m-0 px-8 text-sm text-content-primary">
				Contact our sales team to extend your trial or upgrade to Coder Premium.
			</p>

			<Button asChild className="w-full">
				<a href={CONTACT_SALES_LINK} target="_blank" rel="noreferrer">
					Contact sales
				</a>
			</Button>
		</div>
	);
};
