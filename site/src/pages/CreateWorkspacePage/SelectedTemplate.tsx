import { type FC } from "react";
import type { Template, TemplateExample } from "api/typesGenerated";
import { ExternalAvatar } from "components/Avatar/Avatar";
import { Stack } from "components/Stack/Stack";
import { type Interpolation, type Theme } from "@emotion/react";

export interface SelectedTemplateProps {
  template: Template | TemplateExample;
}

export const SelectedTemplate: FC<SelectedTemplateProps> = ({ template }) => {
  return (
    <Stack
      direction="row"
      spacing={3}
      css={styles.template}
      alignItems="center"
    >
      <ExternalAvatar
        variant={template.icon ? "square" : undefined}
        fitImage={Boolean(template.icon)}
        src={template.icon}
      >
        {template.name}
      </ExternalAvatar>

      <Stack direction="column" spacing={0}>
        <span css={styles.templateName}>
          {"display_name" in template && template.display_name.length > 0
            ? template.display_name
            : template.name}
        </span>
        {template.description && (
          <span css={styles.templateDescription}>{template.description}</span>
        )}
      </Stack>
    </Stack>
  );
};

const styles = {
  template: (theme) => ({
    padding: "20px 24px",
    borderRadius: 8,
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
  }),

  templateName: {
    fontSize: 16,
  },

  templateDescription: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
