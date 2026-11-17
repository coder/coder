import type { LucideIcon } from "lucide-react";
import { useId } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { OverviewCard } from "./OverviewCard";

type StatCardProps = {
	icon: LucideIcon;
	label: string;
	/** Undefined while the figure loads. */
	value: string | undefined;
	children?: React.ReactNode;
};

/** A labeled figure on the overview tab. */
export const StatCard: React.FC<StatCardProps> = ({
	icon: Icon,
	label,
	value,
	children,
}) => {
	const labelId = useId();
	return (
		<OverviewCard aria-labelledby={labelId}>
			<h2
				id={labelId}
				className="m-0 flex items-center gap-2 text-base font-normal text-content-secondary"
			>
				<Icon aria-hidden className="size-icon-sm" />
				{label}
			</h2>
			{value === undefined ? (
				<Skeleton className="h-8 w-40" />
			) : (
				<p className="m-0 text-2xl font-semibold tabular-nums text-content-primary">
					{value}
				</p>
			)}
			{children}
		</OverviewCard>
	);
};
