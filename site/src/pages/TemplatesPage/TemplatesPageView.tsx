import Link from "@material-ui/core/Link"
import { fade, makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import { createDayString } from "util/createDayString"
import * as TypesGen from "../../api/typesGenerated"
import { AvatarData } from "../../components/AvatarData/AvatarData"
import { CodeExample } from "../../components/CodeExample/CodeExample"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import { Margins } from "../../components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { TableCellLink } from "../../components/TableCellLink/TableCellLink"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "../../components/Tooltips/HelpTooltip/HelpTooltip"

export const Language = {
  developerCount: (ownerCount: number): string => {
    return `${ownerCount} developer${ownerCount !== 1 ? "s" : ""}`
  },
  nameLabel: "Name",
  usedByLabel: "Used by",
  lastUpdatedLabel: "Last updated",
  emptyViewNoPerms:
    "Contact your Coder administrator to create a template. You can share the code below.",
  emptyMessage: "Create your first template",
  emptyDescription: (
    <>
      To create a workspace you need to have a template. You can{" "}
      <Link target="_blank" href="https://github.com/coder/coder/blob/main/docs/templates.md">
        create one from scratch
      </Link>{" "}
      or use a built-in template using the following Coder CLI command:
    </>
  ),
  templateTooltipTitle: "What is template?",
  templateTooltipText:
    "With templates you can create a common configuration for your workspaces using Terraform.",
  templateTooltipLink: "Manage templates",
  createdByLabel: "Created by",
}

const TemplateHelpTooltip: React.FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.templateTooltipTitle}</HelpTooltipTitle>
      <HelpTooltipText>{Language.templateTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/templates.md#manage-templates">
          {Language.templateTooltipLink}
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}

export interface TemplatesPageViewProps {
  loading?: boolean
  canCreateTemplate?: boolean
  templates?: TypesGen.Template[]
}

export const TemplatesPageView: FC<React.PropsWithChildren<TemplatesPageViewProps>> = (props) => {
  const styles = useStyles()
  const navigate = useNavigate()

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>
          <Stack spacing={1} direction="row" alignItems="center">
            Templates
            <TemplateHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        {props.templates && props.templates.length > 0 && (
          <PageHeaderSubtitle>Choose a template to create a new workspace.</PageHeaderSubtitle>
        )}
      </PageHeader>

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>{Language.nameLabel}</TableCell>
              <TableCell>{Language.usedByLabel}</TableCell>
              <TableCell>{Language.lastUpdatedLabel}</TableCell>
              <TableCell>{Language.createdByLabel}</TableCell>
              <TableCell width="1%"></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {props.loading && <TableLoader />}
            {!props.loading && !props.templates?.length && (
              <TableRow>
                <TableCell colSpan={999}>
                  <EmptyState
                    message={Language.emptyMessage}
                    description={
                      props.canCreateTemplate
                        ? Language.emptyDescription
                        : Language.emptyViewNoPerms
                    }
                    descriptionClassName={styles.emptyDescription}
                    cta={<CodeExample code="coder template init" />}
                  />
                </TableCell>
              </TableRow>
            )}
            {props.templates?.map((template) => {
              const templatePageLink = `/templates/${template.name}`
              return (
                <TableRow
                  key={template.id}
                  hover
                  data-testid={`template-${template.id}`}
                  tabIndex={0}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      navigate(templatePageLink)
                    }
                  }}
                  className={styles.clickableTableRow}
                >
                  <TableCellLink to={templatePageLink}>
                    <AvatarData
                      title={template.name}
                      subtitle={template.description}
                      highlightTitle
                    />
                  </TableCellLink>

                  <TableCellLink to={templatePageLink}>
                    {Language.developerCount(template.workspace_owner_count)}
                  </TableCellLink>

                  <TableCellLink data-chromatic="ignore" to={templatePageLink}>
                    {createDayString(template.updated_at)}
                  </TableCellLink>
                  <TableCellLink to={templatePageLink}>{template.created_by_name}</TableCellLink>
                  <TableCellLink to={templatePageLink}>
                    <div className={styles.arrowCell}>
                      <KeyboardArrowRight className={styles.arrowRight} />
                    </div>
                  </TableCellLink>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </TableContainer>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  emptyDescription: {
    maxWidth: theme.spacing(62),
  },
  clickableTableRow: {
    "&:hover td": {
      backgroundColor: fade(theme.palette.primary.dark, 0.1),
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    width: 20,
    height: 20,
  },
  arrowCell: {
    display: "flex",
  },
}))
