import Link from "@material-ui/core/Link"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Trans, useTranslation } from "react-i18next"
import * as TypesGen from "api/typesGenerated"

export interface UpdateCheckBannerProps {
  updateCheck?: TypesGen.UpdateCheckResponse
  error?: Error | unknown
  onDismiss?: () => void
}

export const UpdateCheckBanner: React.FC<
  React.PropsWithChildren<UpdateCheckBannerProps>
> = ({ updateCheck, error, onDismiss }) => {
  const { t } = useTranslation("common")

  return (
    <>
      {!error && updateCheck && !updateCheck.current && (
        <AlertBanner severity="info" onDismiss={onDismiss} dismissible>
          <div>
            <Trans
              t={t}
              i18nKey="updateCheck.message"
              values={{ version: updateCheck.version }}
            >
              Coder {"{{version}}"} is now available. View the{" "}
              <Link href={updateCheck.url}>release notes</Link> and{" "}
              <Link href="https://coder.com/docs/coder-oss/latest/admin/upgrade">
                upgrade instructions
              </Link>{" "}
              for more information.
            </Trans>
          </div>
        </AlertBanner>
      )}
      {error && (
        <AlertBanner
          severity="error"
          error={error}
          text={t("updateCheck.error")}
          onDismiss={onDismiss}
          dismissible
        />
      )}
    </>
  )
}
