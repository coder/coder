import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { maxWidth, sidePadding } from "../../theme/constants"

const useStyles = makeStyles(() => ({
  margins: {
    margin: "0 auto",
    maxWidth,
    padding: `0 ${sidePadding}`,
    flex: 1,
    width: "100%",
  },
}))

export const Margins: FC = ({ children }) => {
  const styles = useStyles()
  return <div className={styles.margins}>{children}</div>
}
