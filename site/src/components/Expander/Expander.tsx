import Link from "@material-ui/core/Link"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { CloseDropdown, OpenDropdown } from "components/DropdownArrows/DropdownArrows"

const Language = {
  expand: "More",
  collapse: "Less",
}

export interface ExpanderProps {
  expanded: boolean
  setExpanded: (val: boolean) => void
}

export const Expander: React.FC<ExpanderProps> = ({ expanded, setExpanded }) => {
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
    color: theme.palette.text.primary,
    display: "flex",
  },
  text: {
    display: "flex",
    alignItems: "center",
  },
}))
