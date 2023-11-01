import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { ComponentProps, FC } from "react";
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm";
import { Workspace } from "api/typesGenerated";

export type WorkspaceSettingsPageViewProps = {
  error: unknown;
  workspace: Workspace;
  onCancel: () => void;
  onSubmit: ComponentProps<typeof WorkspaceSettingsForm>["onSubmit"];
  templatePoliciesEnabled: boolean;
};

export const WorkspaceSettingsPageView: FC<WorkspaceSettingsPageViewProps> = ({
  onCancel,
  onSubmit,
  error,
  workspace,
  templatePoliciesEnabled,
}) => {
  return (
    <>
      <PageHeader
        css={{
          paddingTop: 0,
        }}
      >
        <PageHeaderTitle>Workspace Settings</PageHeaderTitle>
      </PageHeader>

      <WorkspaceSettingsForm
        error={error}
        workspace={workspace}
        onCancel={onCancel}
        onSubmit={onSubmit}
        templatePoliciesEnabled={templatePoliciesEnabled}
      />
    </>
  );
};
