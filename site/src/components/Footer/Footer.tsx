import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AccountTreeIcon from "@material-ui/icons/AccountTree"
import AssistantIcon from "@material-ui/icons/Assistant"
import ChatIcon from "@material-ui/icons/Chat"
import { colors } from "theme/colors"
import * as TypesGen from "../../api/typesGenerated"

export const Language = {
  buildInfoText: (buildInfo: TypesGen.BuildInfoResponse): string => {
    return `Coder ${buildInfo.version}`
  },
  copyrightText: `Copyright \u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`,
  reportBugLink: "Report a bug encountered while using Coder",
  enhancementLink: "Request an enhancement to Coder",
  shareFeedbackLink: "Share your experience with Coder",
  discordLink: "Join Coder on Discord",
}

export interface FooterProps {
  buildInfo?: TypesGen.BuildInfoResponse
}

export const Footer: React.FC<React.PropsWithChildren<FooterProps>> = ({
  buildInfo,
}) => {
  const styles = useFooterStyles()


  const githubUrl = 'https://github.com/coder/coder/issues/new?'
  const body = encodeURIComponent(`Version: [\`${buildInfo?.version}\`](${buildInfo?.external_url})`)

  const reportBugUrl = githubUrl + `labels=bug&body=${body}`
  const enhancementUrl = githubUrl + `labels=enhancement&body=${body}`
  const shareFeedbackUrl = githubUrl + `labels=feedback&body=${body}`

  const discordUrl = 'https://coder.com/chat?utm_source=coder&utm_medium=coder&utm_campaign=server-footer'

  return (
    <div className={styles.root}>
      <div className={styles.copyRight}>{Language.copyrightText}</div>
      {buildInfo && (
        <div className={styles.buildInfo}>

          <Link
            className={styles.link}
            variant="caption"
            target="_blank"
            href={buildInfo.external_url}
          >
            <AccountTreeIcon className={styles.icon} />{" "}
            {Language.buildInfoText(buildInfo)}
          </Link>
          &nbsp;|&nbsp;

          <Link
            className={styles.link}
            variant="caption"
            target="_blank"
            href={reportBugUrl}
          >
            <AssistantIcon className={styles.icon} /> {Language.reportBugLink}
          </Link>
          &nbsp;|&nbsp;

          <Link
            className={styles.link}
            variant="caption"
            target="_blank"
            href={enhancementUrl}
          >
            <AssistantIcon className={styles.icon} /> {Language.enhancementLink}
          </Link>
          &nbsp;|&nbsp;

          <Link
            className={styles.link}
            variant="caption"
            target="_blank"
            href={shareFeedbackUrl}
          >
            <AssistantIcon className={styles.icon} /> {Language.shareFeedbackLink}
          </Link>
          &nbsp;|&nbsp;

          <Link
            className={styles.link}
            variant="caption"
            target="_blank"
            href={discordUrl}
          >
            <ChatIcon className={styles.icon} /> {Language.discordLink}
          </Link>
        </div>
      )}
    </div>
  )
}

const useFooterStyles = makeStyles((theme) => ({
  root: {
    color: colors.gray[7],
    textAlign: "center",
    flex: "0",
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
    marginTop: theme.spacing(8),
  },
  copyRight: {
    margin: theme.spacing(0.25),
  },
  buildInfo: {
    margin: theme.spacing(0.25),
    display: "inline-flex",
  },
  link: {
    color: theme.palette.text.secondary,
    fontWeight: 600,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },
  icon: {
    fontSize: 12,
    color: theme.palette.secondary.dark,
    marginRight: theme.spacing(0.5),
  },
}))
