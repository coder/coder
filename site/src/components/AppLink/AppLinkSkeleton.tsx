import { makeStyles } from "@material-ui/core/styles"
import { Skeleton } from "@material-ui/lab"
import { FC } from "react"
import { borderRadiusSm } from "theme/constants"

export const AppLinkSkeleton: FC<{ width: number }> = ({ width }) => {
  const styles = useStyles()
  return (
    <Skeleton
      width={width}
      height={36}
      variant="rect"
      className={styles.skeleton}
    />
  )
}

export const useStyles = makeStyles(() => ({
  skeleton: {
    borderRadius: borderRadiusSm,
  },
}))
