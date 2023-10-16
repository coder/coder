import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { ComponentProps, FC } from "react";
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm";
import { Workspace } from "api/typesGenerated";

export type WorkspaceSettingsPageViewProps = {
  error: unknown;
  isSubmitting: boolean;
  workspace: Workspace;
  onCancel: () => void;
  onSubmit: ComponentProps<typeof WorkspaceSettingsForm>["onSubmit"];
};

export const WorkspaceSettingsPageView: FC<WorkspaceSettingsPageViewProps> = ({
  onCancel,
  onSubmit,
  isSubmitting,
  error,
  workspace,
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
        isSubmitting={isSubmitting}
        workspace={workspace}
        onCancel={onCancel}
        onSubmit={onSubmit}
      />
    </>
  );
};
