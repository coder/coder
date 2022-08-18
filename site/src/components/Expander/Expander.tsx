import Link from "@material-ui/core/Link"
import { lighten } from "@material-ui/core/styles"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { CloseDropdown, OpenDropdown } from "components/DropdownArrows/DropdownArrows"

const Language = {
  expand: "More",
  collapse: "Less",
}

export interface CollapseButtonProps {
  expanded: boolean
  setExpanded: (val: boolean) => void
}

export const Expander: React.FC<CollapseButtonProps> = ({ expanded, setExpanded }) => {
  const toggleExpanded = () => setExpanded(!expanded)
  const styles = useStyles()
  return (
    <Link
      aria-expanded={expanded}
      onClick={toggleExpanded}
      className={styles.expandLink}
      tabIndex={0}
    >
      {expanded ? (
        <span className={styles.text}>
          {Language.collapse}
          <CloseDropdown margin={false} />{" "}
        </span>
      ) : (
        <span className={styles.text}>
          {Language.expand}
          <OpenDropdown margin={false} />
        </span>
      )}
    </Link>
  )
}

const useStyles = makeStyles((theme) => ({
  expandLink: {
    cursor: "pointer",
    color: `${lighten(theme.palette.primary.light, 0.2)}`,
    display: "flex",
  },
  text: {
    display: "flex",
    alignItems: "center",
  },
}))
