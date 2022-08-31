import Collapse from "@material-ui/core/Collapse"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/api"
import { Template, Workspace } from "api/typesGenerated"
import { CodeExample } from "components/CodeExample/CodeExample"
import { CloseDropdown, OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { Margins } from "components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { AuditHelpTooltip } from "components/Tooltips"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { FC, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import { colors } from "theme/colors"
import { combineClasses } from "util/combineClasses"
import { createDayString } from "util/createDayString"

const getDiffValue = (value: number | string | boolean) => {
  if (typeof value === "string") {
    return `"${value}"`
  }

  return value.toString()
}

const AuditDiff: React.FC<{ diff: AuditLog["diff"] }> = ({ diff }) => {
  const styles = useStyles()
  const diffEntries = Object.entries(diff)

  return (
    <div className={styles.diff}>
      <div className={combineClasses([styles.diffColumn, styles.diffOld])}>
        {diffEntries.map(([attrName, valueDiff], index) => (
          <div key={attrName} className={styles.diffRow}>
            <div className={styles.diffLine}>{index + 1}</div>
            <div className={styles.diffIcon}>-</div>
            <div className={styles.diffContent}>
              {attrName}:{" "}
              <span className={combineClasses([styles.diffValue, styles.diffValueOld])}>
                {getDiffValue(valueDiff.old)}
              </span>
            </div>
          </div>
        ))}
      </div>
      <div className={combineClasses([styles.diffColumn, styles.diffNew])}>
        {diffEntries.map(([attrName, valueDiff], index) => (
          <div key={attrName} className={styles.diffRow}>
            <div className={styles.diffLine}>{index + 1}</div>
            <div className={styles.diffIcon}>+</div>
            <div className={styles.diffContent}>
              {attrName}:{" "}
              <span className={combineClasses([styles.diffValue, styles.diffValueNew])}>
                {getDiffValue(valueDiff.new)}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

const getResourceLabel = (resource: AuditLog["resource"]): string => {
  if ("name" in resource) {
    return resource.name
  }

  return resource.username
}

const getResourceHref = (
  resource: AuditLog["resource"],
  resourceType: AuditLog["resource_type"],
): string | undefined => {
  switch (resourceType) {
    case "user":
      return `/users`
    case "template":
      return `/templates/${(resource as Template).name}`
    case "workspace":
      return `/workspaces/@${(resource as Workspace).owner_name}/${(resource as Workspace).name}`
    case "organization":
      return
  }
}

const ResourceLink: React.FC<{
  resource: AuditLog["resource"]
  resourceType: AuditLog["resource_type"]
}> = ({ resource, resourceType }) => {
  const href = getResourceHref(resource, resourceType)
  const label = <strong>{getResourceLabel(resource)}</strong>

  if (!href) {
    return label
  }

  return (
    <Link component={RouterLink} to={href}>
      {label}
    </Link>
  )
}

const actionLabelByAction: Record<AuditLog["action"], string> = {
  create: "created",
  write: "updated",
  delete: "deleted",
}

const resourceLabelByResourceType: Record<AuditLog["resource_type"], string> = {
  organization: "organization",
  template: "template",
  template_version: "template version",
  user: "user",
  workspace: "workspace",
}

const readableActionMessage = (auditLog: AuditLog) => {
  return `${actionLabelByAction[auditLog.action]} ${
    resourceLabelByResourceType[auditLog.resource_type]
  }`
}

const AuditLogRow: React.FC<{ auditLog: AuditLog }> = ({ auditLog }) => {
  const styles = useStyles()
  const [isDiffOpen, setIsDiffOpen] = useState(false)
  const diffs = Object.entries(auditLog.diff)
  const shouldDisplayDiff = diffs.length > 0

  const toggle = () => {
    if (shouldDisplayDiff) {
      setIsDiffOpen((v) => !v)
    }
  }

  return (
    <TableRow key={auditLog.id} hover={shouldDisplayDiff}>
      <TableCell className={styles.auditLogCell}>
        <Stack
          style={{ cursor: shouldDisplayDiff ? "pointer" : undefined }}
          direction="row"
          alignItems="center"
          className={styles.auditLogRow}
          tabIndex={0}
          onClick={toggle}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              toggle()
            }
          }}
        >
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            className={styles.auditLogRowInfo}
          >
            <Stack direction="row" alignItems="center">
              <UserAvatar username={auditLog.user?.username ?? ""} />
              <div>
                <span className={styles.auditLogResume}>
                  <strong>{auditLog.user?.username}</strong> {readableActionMessage(auditLog)}{" "}
                  <ResourceLink
                    resource={auditLog.resource}
                    resourceType={auditLog.resource_type}
                  />
                </span>
                <span className={styles.auditLogTime}>{createDayString(auditLog.time)}</span>
              </div>
            </Stack>

            <Stack direction="column" alignItems="flex-end" spacing={1}>
              <Pill type="success" text={auditLog.status_code.toString()} />
              <Stack direction="row" alignItems="center" className={styles.auditLogExtraInfo}>
                <div>
                  <strong>IP</strong> {auditLog.ip}
                </div>
                <div>
                  <strong>Agent</strong> {auditLog.user_agent}
                </div>
              </Stack>
            </Stack>
          </Stack>

          <div className={shouldDisplayDiff ? undefined : styles.disabledDropdownIcon}>
            {isDiffOpen ? <CloseDropdown /> : <OpenDropdown />}
          </div>
        </Stack>

        {shouldDisplayDiff && (
          <Collapse in={isDiffOpen}>
            <AuditDiff diff={auditLog.diff} />
          </Collapse>
        )}
      </TableCell>
    </TableRow>
  )
}

export const Language = {
  title: "Audit",
  subtitle: "View events in your audit log.",
  tooltipTitle: "Copy to clipboard and try the Coder CLI",
}

export const AuditPageView: FC<{ auditLogs?: AuditLog[] }> = ({ auditLogs }) => {
  return (
    <Margins>
      <PageHeader
        actions={
          <CodeExample tooltipTitle={Language.tooltipTitle} code="coder audit [organization_ID]" />
        }
      >
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.title}</span>
            <AuditHelpTooltip />
          </Stack>
        </PageHeaderTitle>
        <PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
      </PageHeader>

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Logs</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {auditLogs ? (
              auditLogs.map((auditLog) => <AuditLogRow auditLog={auditLog} key={auditLog.id} />)
            ) : (
              <TableLoader />
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  auditLogCell: {
    padding: "0 !important",
  },

  auditLogRow: {
    padding: theme.spacing(2, 4),
  },

  auditLogRowInfo: {
    flex: 1,
  },

  auditLogResume: {
    ...theme.typography.body1,
    fontFamily: "inherit",
    display: "block",
  },

  auditLogTime: {
    ...theme.typography.body2,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  },

  auditLogExtraInfo: {
    ...theme.typography.body2,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
  },

  disabledDropdownIcon: {
    opacity: 0.5,
  },

  diff: {
    display: "flex",
    alignItems: "flex-start",
    fontSize: theme.typography.body2.fontSize,
    borderTop: `1px solid ${theme.palette.divider}`,
  },

  diffColumn: {
    flex: 1,
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2.5),
    lineHeight: "160%",
  },

  diffOld: {
    backgroundColor: theme.palette.error.dark,
    color: theme.palette.error.contrastText,
  },

  diffRow: {
    display: "flex",
    alignItems: "baseline",
  },

  diffLine: {
    opacity: 0.5,

    width: theme.spacing(8),
    textAlign: "right",
  },

  diffIcon: {
    width: theme.spacing(4),
    textAlign: "center",
    fontSize: theme.typography.body1.fontSize,
  },

  diffContent: {},

  diffNew: {
    backgroundColor: theme.palette.success.dark,
    color: theme.palette.success.contrastText,
  },

  diffValue: {
    padding: 1,
    borderRadius: theme.shape.borderRadius / 2,
  },

  diffValueOld: {
    backgroundColor: colors.red[12],
  },

  diffValueNew: {
    backgroundColor: colors.green[12],
  },
}))
