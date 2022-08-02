import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { containerWidth, sidePadding } from "../../theme/constants"

type Size = "regular" | "medium" | "small"

const widthBySize: Record<Size, number> = {
  regular: containerWidth,
  medium: containerWidth / 2,
  small: containerWidth / 3,
}

const useStyles = makeStyles(() => ({
  margins: {
    margin: "0 auto",
    maxWidth: ({ maxWidth }: { maxWidth: number }) => maxWidth,
    padding: `0 ${sidePadding}px`,
    flex: 1,
    width: "100%",
  },
}))

interface MarginsProps {
  size?: Size
}

export const Margins: FC<React.PropsWithChildren<MarginsProps>> = ({ children, size = "regular" }) => {
  const styles = useStyles({ maxWidth: widthBySize[size] })
  return <div className={styles.margins}>{children}</div>
}
