import { FC } from "react"
import * as TypesGen from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Maybe } from "components/Conditionals/Maybe"
import Link from "@mui/material/Link"

export interface TemplateVersionWarningsProps {
  warnings?: TypesGen.TemplateVersionWarning[]
}

export const TemplateVersionWarnings: FC<
  React.PropsWithChildren<TemplateVersionWarningsProps>
> = ({ warnings }) => {
  if (!warnings) {
    return <></>
  }

  return (
    <Maybe condition={Boolean(warnings.includes("DEPRECATED_PARAMETERS"))}>
      <div data-testid="warning-deprecated-parameters">
        <AlertBanner severity="warning">
          <div>
            This template uses legacy parameters which will be deprecated in the
            next Coder release. Learn how to migrate in{" "}
            <Link href="https://coder.com/docs/v2/latest/templates/parameters#migration">
              our documentation
            </Link>
            .
          </div>
        </AlertBanner>
      </div>
    </Maybe>
  )
}
