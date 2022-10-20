import Link from "@material-ui/core/Link"
import makeStyles from "@material-ui/core/styles/makeStyles"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { PropsWithChildren, FC } from "react"
import Collapse from "@material-ui/core/Collapse"
import { useTranslation } from "react-i18next"
import { combineClasses } from "util/combineClasses"

export interface ExpanderProps {
  expanded: boolean
  setExpanded: (val: boolean) => void
}

export const Expander: FC<PropsWithChildren<ExpanderProps>> = ({
  expanded,
  setExpanded,
  children,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("common")

  const toggleExpanded = () => setExpanded(!expanded)

  return (
    <>
      {!expanded && (
        <Link onClick={toggleExpanded} className={styles.expandLink}>
          <span className={styles.text}>
            {t("ctas.expand")}
            <OpenDropdown margin={false} />
          </span>
        </Link>
      )}
      <Collapse in={expanded}>
        <div className={styles.text}>{children}</div>
      </Collapse>
      {expanded && (
        <Link
          onClick={toggleExpanded}
          className={combineClasses([styles.expandLink, styles.collapseLink])}
        >
          <span className={styles.text}>
            {t("ctas.collapse")}
            <CloseDropdown margin={false} />
          </span>
        </Link>
      )}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  expandLink: {
    cursor: "pointer",
    color: theme.palette.text.secondary,
  },
  collapseLink: {
    marginTop: `${theme.spacing(2)}px`,
  },
  text: {
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontSize: theme.typography.caption.fontSize,
  },
}))
