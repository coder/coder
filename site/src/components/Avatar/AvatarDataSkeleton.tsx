import Skeleton from "@mui/material/Skeleton";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";

export const AvatarDataSkeleton: FC = () => {
	return (
		<Stack spacing={1} direction="row" className="w-full">
			<Skeleton variant="rectangular" className="size-6 rounded-sm" />

			<Stack spacing={0}>
				<Skeleton variant="text" width={100} />
				<Skeleton variant="text" width={60} />
			</Stack>
		</Stack>
	);
};
