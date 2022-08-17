import { makeStyles, darken } from "@material-ui/core/styles"
import { Pill } from "components/Pill/Pill"

const Language = {
  licenseIssue: "License Issue",
  licenseIssues: (num: number) => `${num} License Issues`
}

export interface LicenseBannerViewProps {
  warnings?: string[]
}

export const LicenseBannerView: React.FC<LicenseBannerViewProps> = ({ warnings }) => {
  const styles = useStyles()
  if (warnings && warnings.length) {
    if (warnings.length === 1) {
      return (
        <div className={styles.container}>
          <Pill text={Language.licenseIssue} type="warning" />
          {warnings[0]}
        </div>
      )
    } else {
      return (
        <div className={styles.container}>
          <Pill text={Language.licenseIssues(warnings.length)} type="warning" />
          {warnings.map((warning, i) => (
            <p key={`${i}`}>{warning}</p>
          ))}
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
    backgroundColor: darken(theme.palette.warning.main, .2)
  }
}))
