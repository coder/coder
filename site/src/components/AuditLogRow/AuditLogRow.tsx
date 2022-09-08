import Collapse from "@material-ui/core/Collapse"
import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { AuditLog } from "api/typesGenerated"
import { CloseDropdown, OpenDropdown } from "components/DropdownArrows/DropdownArrows"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { t } from "i18next"
import { ComponentProps, useState } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import userAgentParser from "ua-parser-js"
import { createDayString } from "util/createDayString"
import { AuditLogDiff } from "./AuditLogDiff"

const pillTypeByHttpStatus = (httpStatus: number): ComponentProps<typeof Pill>["type"] => {
  if (httpStatus >= 300 && httpStatus < 500) {
    return "warning"
  }

  if (httpStatus >= 500) {
    return "error"
  }

  return "success"
}

const actionLabelByAction: Record<AuditLog["action"], string> = {
  create: t("actions.create", { ns: "auditLog" }),
  write: t("actions.write", { ns: "auditLog" }),
  delete: t("actions.delete", { ns: "auditLog" }),
}

const resourceLabelByResourceType: Record<AuditLog["resource_type"], string> = {
  organization: "organization",
  template: "template",
  template_version: "template version",
  user: "user",
  workspace: "workspace",
  git_ssh_key: "git ssh key",
  api_key: "api key",
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
  const userAgent = userAgentParser(auditLog.user_agent)

  const toggle = () => {
    if (shouldDisplayDiff) {
      setIsDiffOpen((v) => !v)
    }
  }

  return (
    <TableRow key={auditLog.id} data-testid={`audit-log-row-${auditLog.id}`}>
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
              <UserAvatar
                username={auditLog.user?.username ?? ""}
                avatarURL={auditLog.user?.avatar_url}
              />
              <div>
                <span className={styles.auditLogResume}>
                  <strong>{auditLog.user?.username}</strong> {readableActionMessage(auditLog)}{" "}
                  <strong>{auditLog.resource_target}</strong>
                </span>
                <span className={styles.auditLogTime}>{createDayString(auditLog.time)}</span>
              </div>
            </Stack>

            <Stack
              direction="column"
              alignItems="flex-end"
              spacing={1}
              className={styles.auditLogRight}
            >
              <Pill
                type={pillTypeByHttpStatus(auditLog.status_code)}
                text={auditLog.status_code.toString()}
              />
              <Stack direction="row" alignItems="center" className={styles.auditLogExtraInfo}>
                <div>
                  <strong>IP</strong> {auditLog.ip}
                </div>
                <div>
                  <strong>OS</strong> {userAgent.os.name}
                </div>
                <div>
                  <strong>Browser</strong> {userAgent.browser.name} {userAgent.browser.version}
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
            <AuditLogDiff diff={auditLog.diff} />
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

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
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
    fontSize: 12,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  },

  auditLogRight: {
    width: "auto",
  },

  auditLogExtraInfo: {
    ...theme.typography.body2,
    fontFamily: MONOSPACE_FONT_FAMILY,
    color: theme.palette.text.secondary,
    whiteSpace: "nowrap",
  },

  disabledDropdownIcon: {
    opacity: 0.5,
  },
}))
