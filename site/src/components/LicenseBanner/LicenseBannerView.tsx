import Collapse from "@material-ui/core/Collapse"
import { darken, makeStyles } from "@material-ui/core/styles"
import { Expander } from "components/Expander/Expander"
import { Pill } from "components/Pill/Pill"
import { useState } from "react"

const Language = {
  licenseIssue: "License Issue",
  licenseIssues: (num: number) => `${num} License Issues`,
  upgrade: "Contact us to upgrade your license.",
  exceeded: "It looks like you've exceeded some limits of your license.",
  lessDetails: "Less",
  moreDetails: "More",
}

export interface LicenseBannerViewProps {
  warnings?: string[]
}

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({ warnings }) => {
  const styles = useStyles()
  const [showDetails, setShowDetails] = useState(false)
  if (warnings && warnings.length) {
    if (warnings.length === 1) {
      return (
        <div className={styles.container}>
          <Pill text={Language.licenseIssue} type="warning" />
          <span className={styles.text}>{warnings[0]}</span>
          &nbsp;
          <a href="mailto:sales@coder.com" className={styles.link}>
            {Language.upgrade}
          </a>
        </div>
      )
    } else {
      return (
        <div className={styles.container}>
          <div className={styles.flex}>
            <div className={styles.leftContent}>
              <Pill text={Language.licenseIssues(warnings.length)} type="warning" />
              <span className={styles.text}>{Language.exceeded}</span>
              &nbsp;
              <a href="mailto:sales@coder.com" className={styles.link}>
                {Language.upgrade}
              </a>
            </div>
            <Expander expanded={showDetails} setExpanded={setShowDetails} />
          </div>
          <Collapse in={showDetails}>
            <ul className={styles.list}>
              {warnings.map((warning, i) => (
                <li className={styles.listItem} key={`${i}-${warning}`}>
                  {warning}
                </li>
              ))}
            </ul>
          </Collapse>
        </div>
      )
    }
  } else {
    return null
  }
}

const useStyles = makeStyles((theme) => ({
  container: {
    padding: theme.spacing(1.5),
    backgroundColor: darken(theme.palette.warning.main, 0.2),
  },
  flex: {
    display: "flex",
    justifyContent: "space-between",
  },
  leftContent: {
    marginRight: theme.spacing(1),
  },
  text: {
    marginLeft: theme.spacing(1),
  },
  link: {
    color: "inherit",
    textDecoration: "none",
    fontWeight: "bold",
  },
  list: {
    margin: theme.spacing(1.5),
  },
  listItem: {
    margin: theme.spacing(1),
  },
}))
