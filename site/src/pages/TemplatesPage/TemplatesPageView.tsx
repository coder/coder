import Link from "@material-ui/core/Link"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import useTheme from "@material-ui/styles/useTheme"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Maybe } from "components/Conditionals/Maybe"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import { createDayString } from "util/createDayString"
import { formatTemplateActiveDevelopers } from "util/templates"
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
  developerCount: (activeCount: number): string => {
    return `${formatTemplateActiveDevelopers(activeCount)} developer${
      activeCount !== 1 ? "s" : ""
    }`
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
      <Link
        target="_blank"
        href="https://coder.com/docs/coder-oss/latest/templates"
      >
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
        <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/templates#manage-templates">
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
  getOrganizationsError?: Error | unknown
  getTemplatesError?: Error | unknown
}

export const TemplatesPageView: FC<
  React.PropsWithChildren<TemplatesPageViewProps>
> = (props) => {
  const styles = useStyles()
  const navigate = useNavigate()
  const { t } = useTranslation("templatesPage")
  const theme: Theme = useTheme()
  const empty =
    !props.loading &&
    !props.getOrganizationsError &&
    !props.getTemplatesError &&
    !props.templates?.length

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>
          <Stack spacing={1} direction="row" alignItems="center">
            Templates
            <TemplateHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        <Maybe
          condition={Boolean(props.templates && props.templates.length > 0)}
        >
          <PageHeaderSubtitle>
            Choose a template to create a new workspace
            {props.canCreateTemplate ? (
              <>
                , or{" "}
                <Link
                  href="https://coder.com/docs/coder-oss/latest/templates#add-a-template"
                  target="_blank"
                >
                  manage templates
                </Link>{" "}
                from the CLI.
              </>
            ) : (
              "."
            )}
          </PageHeaderSubtitle>
        </Maybe>
      </PageHeader>

      <ChooseOne>
        <Cond condition={Boolean(props.getOrganizationsError)}>
          <AlertBanner
            severity="error"
            error={props.getOrganizationsError}
            text={t("errors.getOrganizationsError")}
          />
        </Cond>
        <Cond condition={Boolean(props.getTemplatesError)}>
          <AlertBanner
            severity="error"
            error={props.getTemplatesError}
            text={t("errors.getTemplatesError")}
          />
        </Cond>
        <Cond>
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="50%">{Language.nameLabel}</TableCell>
                  <TableCell width="16%">{Language.usedByLabel}</TableCell>
                  <TableCell width="16%">{Language.lastUpdatedLabel}</TableCell>
                  <TableCell width="16%">{Language.createdByLabel}</TableCell>
                  <TableCell width="1%"></TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                <Maybe condition={Boolean(props.loading)}>
                  <TableLoader />
                </Maybe>

                <ChooseOne>
                  <Cond condition={empty}>
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
                          cta={<CodeExample code="coder templates init" />}
                        />
                      </TableCell>
                    </TableRow>
                  </Cond>
                  <Cond>
                    {props.templates?.map((template) => {
                      const templatePageLink = `/templates/${template.name}`
                      const hasIcon = template.icon && template.icon !== ""

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
                              avatar={
                                hasIcon && (
                                  <div className={styles.templateIconWrapper}>
                                    <img alt="" src={template.icon} />
                                  </div>
                                )
                              }
                            />
                          </TableCellLink>

                          <TableCellLink to={templatePageLink}>
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {Language.developerCount(
                                template.active_user_count,
                              )}
                            </span>
                          </TableCellLink>

                          <TableCellLink
                            data-chromatic="ignore"
                            to={templatePageLink}
                          >
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {createDayString(template.updated_at)}
                            </span>
                          </TableCellLink>

                          <TableCellLink to={templatePageLink}>
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {template.created_by_name}
                            </span>
                          </TableCellLink>

                          <TableCellLink to={templatePageLink}>
                            <div className={styles.arrowCell}>
                              <KeyboardArrowRight
                                className={styles.arrowRight}
                              />
                            </div>
                          </TableCellLink>
                        </TableRow>
                      )
                    })}
                  </Cond>
                </ChooseOne>
              </TableBody>
            </Table>
          </TableContainer>
        </Cond>
      </ChooseOne>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  emptyDescription: {
    maxWidth: theme.spacing(62),
  },
  clickableTableRow: {
    "&:hover td": {
      backgroundColor: theme.palette.action.hover,
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  },
  arrowCell: {
    display: "flex",
  },
  templateIconWrapper: {
    // Same size then the avatar component
    width: 36,
    height: 36,
    padding: 2,

    "& img": {
      width: "100%",
    },
  },
}))
