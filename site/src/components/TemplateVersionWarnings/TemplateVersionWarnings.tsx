import { FC } from "react";
import * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Maybe } from "components/Conditionals/Maybe";

export interface TemplateVersionWarningsProps {
  warnings?: TypesGen.TemplateVersionWarning[];
}

export const TemplateVersionWarnings: FC<
  React.PropsWithChildren<TemplateVersionWarningsProps>
> = ({ warnings }) => {
  if (!warnings) {
    return <></>;
  }

  return (
    <Maybe condition={Boolean(warnings.includes("UNSUPPORTED_WORKSPACES"))}>
      <div data-testid="error-unsupported-workspaces">
        <Alert severity="error">
          This template uses legacy parameters which are not supported anymore.
          Contact your administrator for assistance.
        </Alert>
      </div>
    </Maybe>
  );
};
