import { type ComponentProps, type FC } from "react";
import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { TemplateSettingsForm } from "./TemplateSettingsForm";

export interface TemplateSettingsPageViewProps {
  template: Template;
  onSubmit: (data: UpdateTemplateMeta) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  submitError?: unknown;
  initialTouched?: ComponentProps<
    typeof TemplateSettingsForm
  >["initialTouched"];
  accessControlEnabled: boolean;
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  isSubmitting,
  submitError,
  initialTouched,
  accessControlEnabled,
}) => {
  return (
    <>
      <PageHeader css={{ paddingTop: 0 }}>
        <PageHeaderTitle>General Settings</PageHeaderTitle>
      </PageHeader>

      <TemplateSettingsForm
        initialTouched={initialTouched}
        isSubmitting={isSubmitting}
        template={template}
        onSubmit={onSubmit}
        onCancel={onCancel}
        error={submitError}
        accessControlEnabled={accessControlEnabled}
      />
    </>
  );
};
