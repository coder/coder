import { type CSSObject, type Interpolation, type Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import TableCell from "@mui/material/TableCell";
import { type FC } from "react";
import { useNavigate } from "react-router-dom";
import type { TemplateVersion } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
import { useClickableTableRow } from "hooks/useClickableTableRow";

export interface VersionRowProps {
  version: TemplateVersion;
  isActive: boolean;
  isLatest: boolean;
  onPromoteClick?: (templateVersionId: string) => void;
  onArchiveClick?: (templateVersionId: string) => void;
}

export const VersionRow: FC<VersionRowProps> = ({
  version,
  isActive,
  isLatest,
  onPromoteClick,
  onArchiveClick,
}) => {
  const navigate = useNavigate();

  const clickableProps = useClickableTableRow({
    onClick: () => navigate(version.name),
  });

  const jobStatus = version.job.status;
  const showActions = onPromoteClick || onArchiveClick;

  return (
    <TimelineEntry
      data-testid={`version-${version.id}`}
      {...clickableProps}
      css={[styles.row]}
      className={clickableProps.className}
    >
      <TableCell css={styles.versionCell}>
        <Stack
          direction="row"
          alignItems="center"
          css={styles.versionWrapper}
          justifyContent="space-between"
        >
          <Stack direction="row" alignItems="center">
            <UserAvatar
              username={version.created_by.username}
              avatarURL={version.created_by.avatar_url}
            />
            <Stack
              css={styles.versionSummary}
              direction="row"
              alignItems="center"
              spacing={1}
            >
              <span>
                <strong>{version.created_by.username}</strong> created the
                version <strong>{version.name}</strong>
              </span>

              {version.message && (
                <InfoTooltip title="Message" message={version.message} />
              )}

              <span css={styles.versionTime}>
                {new Date(version.created_at).toLocaleTimeString()}
              </span>
            </Stack>
          </Stack>

          <Stack direction="row" alignItems="center" spacing={2}>
            {isActive && <Pill text="Active" type="success" />}
            {isLatest && <Pill text="Newest" type="info" />}

            {jobStatus === "pending" && (
              <Pill text={<>Pending&hellip;</>} type="warning" />
            )}
            {jobStatus === "running" && (
              <Pill text={<>Building&hellip;</>} type="warning" />
            )}
            {(jobStatus === "canceling" || jobStatus === "canceled") && (
              <Pill text="Canceled" type="neutral" />
            )}
            {jobStatus === "failed" && <Pill text="Failed" type="error" />}

            {showActions && (
              <>
                {jobStatus === "failed" ? (
                  <Button
                    css={styles.promoteButton}
                    disabled={isActive || version.archived}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      onArchiveClick?.(version.id);
                    }}
                  >
                    Archive&hellip;
                  </Button>
                ) : (
                  <Button
                    css={styles.promoteButton}
                    disabled={isActive || jobStatus !== "succeeded"}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      onPromoteClick?.(version.id);
                    }}
                  >
                    Promote&hellip;
                  </Button>
                )}
              </>
            )}
          </Stack>
        </Stack>
      </TableCell>
    </TimelineEntry>
  );
};

const styles = {
  row: (theme) => ({
    "&:hover $promoteButton": {
      color: theme.palette.text.primary,
      borderColor: theme.colors.gray[11],
      "&:hover": {
        borderColor: theme.palette.text.primary,
      },
    },
  }),

  promoteButton: (theme) => ({
    color: theme.palette.text.secondary,
    transition: "none",
  }),

  versionWrapper: {
    padding: "16px 32px",
  },

  versionCell: {
    padding: "0 !important",
    position: "relative",
    borderBottom: 0,
  },

  versionSummary: (theme) => ({
    ...(theme.typography.body1 as CSSObject),
    fontFamily: "inherit",
  }),

  versionTime: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 12,
  }),
} satisfies Record<string, Interpolation<Theme>>;
