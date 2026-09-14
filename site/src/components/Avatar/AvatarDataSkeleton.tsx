import type { FC } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";

export const AvatarDataSkeleton: FC = () => {
	return (
		<div className="flex items-center gap-3 w-full">
			<Skeleton className="size-10 rounded-sm shrink-0" />

			<div className="flex flex-col w-full">
				<Skeleton variant="text" width={100} />
				<Skeleton variant="text" width={60} />
			</div>
		</div>
	);
};
