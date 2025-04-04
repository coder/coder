import Skeleton from "@mui/material/Skeleton";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";

export const AvatarDataSkeleton: FC = () => {
	return (
		<div className="flex items-center gap-3 w-full">
			<Skeleton variant="rectangular" className="size-10 rounded-sm shrink-0" />

			<div className="flex flex-col w-full">
				<Skeleton variant="text" width={100} />
				<Skeleton variant="text" width={60} />
			</div>
		</div>
	);
};
