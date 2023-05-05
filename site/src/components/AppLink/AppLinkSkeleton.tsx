import { makeStyles } from "@mui/material/styles"
import { Skeleton } from '@mui/material';
import { FC } from "react"
import { borderRadiusSm } from "theme/constants"

export const AppLinkSkeleton: FC<{ width: number }> = ({ width }) => {
  const styles = useStyles()
  return (
    <Skeleton
      width={width}
      height={36}
      variant="rectangular"
      className={styles.skeleton}
    />
  );
}

const useStyles = makeStyles(() => ({
  skeleton: {
    borderRadius: borderRadiusSm,
  },
}))
