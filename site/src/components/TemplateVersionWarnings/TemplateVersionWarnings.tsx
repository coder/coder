import { FC } from "react";
import * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";

export interface TemplateVersionWarningsProps {
  warnings?: TypesGen.TemplateVersionWarning[];
}

export const TemplateVersionWarnings: FC<
  React.PropsWithChildren<TemplateVersionWarningsProps>
> = (props) => {
  const { warnings = [] } = props;

  if (!warnings.includes("UNSUPPORTED_WORKSPACES")) {
    return null;
  }

  return (
    <div data-testid="error-unsupported-workspaces">
      <Alert severity="error">
        This template uses legacy parameters which are not supported anymore.
        Contact your administrator for assistance.
      </Alert>
    </div>
  );
};
