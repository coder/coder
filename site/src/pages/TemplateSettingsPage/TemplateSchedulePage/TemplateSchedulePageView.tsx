import { type ComponentProps, type FC } from "react";
import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { TemplateScheduleForm } from "./TemplateScheduleForm";

export interface TemplateSchedulePageViewProps {
  template: Template;
  onSubmit: (data: UpdateTemplateMeta) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  submitError?: unknown;
  initialTouched?: ComponentProps<
    typeof TemplateScheduleForm
  >["initialTouched"];
  allowAdvancedScheduling: boolean;
  allowAutostopRequirement: boolean;
}

export const TemplateSchedulePageView: FC<TemplateSchedulePageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  isSubmitting,
  allowAdvancedScheduling,
  allowAutostopRequirement,
  submitError,
  initialTouched,
}) => {
  return (
    <>
      <PageHeader css={{ paddingTop: 0 }}>
        <PageHeaderTitle>Template schedule</PageHeaderTitle>
      </PageHeader>

      <TemplateScheduleForm
        allowAdvancedScheduling={allowAdvancedScheduling}
        allowAutostopRequirement={allowAutostopRequirement}
        initialTouched={initialTouched}
        isSubmitting={isSubmitting}
        template={template}
        onSubmit={onSubmit}
        onCancel={onCancel}
        error={submitError}
      />
    </>
  );
};
