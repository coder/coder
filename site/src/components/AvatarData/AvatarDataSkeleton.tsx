import { FC } from "react"
import { Stack } from "components/Stack/Stack"
import { Skeleton } from "@material-ui/lab"

export const AvatarDataSkeleton: FC = () => {
  return (
    <Stack spacing={1.5} direction="row" alignItems="center">
      <Skeleton variant="circle" width={36} height={36} />

      <Stack spacing={0}>
        <Skeleton variant="text" width={100} />
        <Skeleton variant="text" width={60} />
      </Stack>
    </Stack>
  )
}
