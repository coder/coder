import Collapse from "@material-ui/core/Collapse"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog, Template, Workspace } from "api/typesGenerated"
import { CloseDropdown, OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { ComponentProps, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import { createDayString } from "util/createDayString"
import { AuditDiff } from "./AuditLogDiff"

const pillTypeByHttpStatus = (httpStatus: number): ComponentProps<typeof Pill>["type"] => {
  if (httpStatus >= 300 && httpStatus < 500) {
    return "warning"
  }

  if (httpStatus > 500) {
    return "error"
  }

  return "success"
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
  create: "created a new",
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

export interface AuditLogRowProps {
  auditLog: AuditLog
  // Useful for Storybook
  defaultIsDiffOpen?: boolean
}

export const AuditLogRow: React.FC<AuditLogRowProps> = ({
  auditLog,
  defaultIsDiffOpen = false,
}) => {
  const styles = useStyles()
  const [isDiffOpen, setIsDiffOpen] = useState(defaultIsDiffOpen)
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
              <Pill
                type={pillTypeByHttpStatus(auditLog.status_code)}
                text={auditLog.status_code.toString()}
              />
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
}))
