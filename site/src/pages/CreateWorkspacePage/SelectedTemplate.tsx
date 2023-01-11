import Avatar from "@material-ui/core/Avatar"
import { makeStyles } from "@material-ui/core/styles"
import { Template, TemplateExample } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { firstLetter } from "util/firstLetter"

export interface SelectedTemplateProps {
  template: Template | TemplateExample
}

export const SelectedTemplate: FC<SelectedTemplateProps> = ({ template }) => {
  const styles = useStyles()

  return (
    <Stack
      direction="row"
      spacing={3}
      className={styles.template}
      alignItems="center"
    >
      <div className={styles.templateIcon}>
        {template.icon === "" ? (
          <Avatar>{firstLetter(template.name)}</Avatar>
        ) : (
          <img src={template.icon} alt="" />
        )}
      </div>
      <Stack direction="column" spacing={0.5}>
        <span className={styles.templateName}>
          {"display_name" in template && template.display_name.length > 0
            ? template.display_name
            : template.name}
        </span>
        {template.description && (
          <span className={styles.templateDescription}>
            {template.description}
          </span>
        )}
      </Stack>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  template: {
    padding: theme.spacing(2.5, 3),
    borderRadius: theme.shape.borderRadius,
    backgroundColor: theme.palette.background.paper,
    border: `1px solid ${theme.palette.divider}`,
  },

  templateName: {
    fontSize: 16,
  },

  templateDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
  },

  templateIcon: {
    width: theme.spacing(4),
    lineHeight: 1,

    "& img": {
      width: "100%",
    },
  },
}))
