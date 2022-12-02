import Link from "@material-ui/core/Link"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Trans, useTranslation } from "react-i18next"
import * as TypesGen from "api/typesGenerated"
import { FC, useState } from "react"

export interface UpdateCheckBannerProps {
  updateCheck?: TypesGen.UpdateCheckResponse
  error?: Error | unknown
  onDismiss?: () => void
}

export const UpdateCheckBanner: FC<
  React.PropsWithChildren<UpdateCheckBannerProps>
> = ({ updateCheck, error, onDismiss }) => {
  const { t } = useTranslation("common")

  const isOutdated = updateCheck && !updateCheck.current

  const [show, setShow] = useState(error || isOutdated)

  const dismiss = () => {
    onDismiss && onDismiss()
    setShow(false)
  }

  return (
    <>
      {show && (
        <AlertBanner
          severity={error ? "error" : "info"}
          error={error}
          onDismiss={dismiss}
          dismissible
        >
          <>
            {error && <>{t("updateCheck.error")} </>}
            {isOutdated && (
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
            )}
          </>
        </AlertBanner>
      )}
    </>
  )
}
