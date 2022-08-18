import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { FC } from "react"
import { TemplateSettingsForm } from "./TemplateSettingsForm"

export const Language = {
  title: "Template settings",
}

export interface TemplateSettingsPageView {
  template?: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageView> = ({
  template,
  onCancel,
  onSubmit,
}) => {
  return (
    <FullPageForm title={Language.title} onCancel={onCancel}>
      {template ? (
        <TemplateSettingsForm template={template} onSubmit={onSubmit} onCancel={onCancel} />
      ) : (
        <Loader />
      )}
    </FullPageForm>
  )
}
