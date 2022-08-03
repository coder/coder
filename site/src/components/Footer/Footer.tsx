import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import AccountTreeIcon from "@material-ui/icons/AccountTree"
import AssistantIcon from "@material-ui/icons/Assistant"
import ChatIcon from "@material-ui/icons/Chat"
import * as TypesGen from "../../api/typesGenerated"

export const Language = {
  buildInfoText: (buildInfo: TypesGen.BuildInfoResponse): string => {
    return `Coder ${buildInfo.version}`
  },
  copyrightText: `Copyright \u00a9 ${new Date().getFullYear()} Coder Technologies, Inc. All rights reserved.`,
  reportBugLink: "Report an issue or share feedback",
  discordLink: "Join Coder on Discord",
}

export interface FooterProps {
  buildInfo?: TypesGen.BuildInfoResponse
}

export const Footer: React.FC<React.PropsWithChildren<FooterProps>> = ({ buildInfo }) => {
  const styles = useFooterStyles()

  const githubUrl = `https://github.com/coder/coder/issues/new?labels=needs+grooming&body=${encodeURIComponent(`Version: [\`${buildInfo?.version}\`](${buildInfo?.external_url})

<!--- Ask a question or leave feedback! -->`)}`
  const discordUrl = `https://discord.gg/coder`

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
            <AccountTreeIcon className={styles.icon} /> {Language.buildInfoText(buildInfo)}
          </Link>
          &nbsp;|&nbsp;
          <Link className={styles.link} variant="caption" target="_blank" href={githubUrl}>
            <AssistantIcon className={styles.icon} /> {Language.reportBugLink}
          </Link>
          &nbsp;|&nbsp;
          <Link className={styles.link} variant="caption" target="_blank" href={discordUrl}>
            <ChatIcon className={styles.icon} /> {Language.discordLink}
          </Link>
        </div>
      )}
    </div>
  )
}

const useFooterStyles = makeStyles((theme) => ({
  root: {
    opacity: 0.6,
    textAlign: "center",
    flex: "0",
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
    marginTop: theme.spacing(3),
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
