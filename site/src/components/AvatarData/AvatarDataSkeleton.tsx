import Skeleton from "@mui/material/Skeleton";
import type { FC } from "react";
import { Stack } from "components/Stack/Stack";

export const AvatarDataSkeleton: FC = () => {
  return (
    <Stack spacing={1.5} direction="row" alignItems="center">
      <Skeleton variant="circular" width={36} height={36} />

      <Stack spacing={0}>
        <Skeleton variant="text" width={100} />
        <Skeleton variant="text" width={60} />
      </Stack>
    </Stack>
  );
};
