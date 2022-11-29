import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Margins } from "components/Margins/Margins"
import { Trans, useTranslation } from "react-i18next"
import * as TypesGen from "api/typesGenerated"

export interface UpdateCheckBannerProps {
  updateCheck?: TypesGen.UpdateCheckResponse
}

export const UpdateCheckBanner: React.FC<
  React.PropsWithChildren<UpdateCheckBannerProps>
> = ({ updateCheck }) => {
  const styles = useStyles({})
  const { t } = useTranslation("common")

  return (
    <div className={styles.root}>
      {updateCheck && !updateCheck.current && (
        <Margins>
          <AlertBanner severity="info" text="" dismissible>
            <div>
              <Trans t={t} i18nKey="updateCheck.message">
                Coder {updateCheck.version} is now available. View the{" "}
                <Link href={updateCheck.url}>release notes</Link> and{" "}
                <Link href="https://coder.com/docs/coder-oss/latest/admin/upgrade">
                  upgrade instructions
                </Link>{" "}
                for more information.
              </Trans>
            </div>
          </AlertBanner>
        </Margins>
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    // Common spacing between elements, adds separation from Navbar.
    paddingTop: theme.spacing(2),
  },
}))
