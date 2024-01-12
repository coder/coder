import PersonOutlinedIcon from "@mui/icons-material/PersonOutlined";
import ScheduleIcon from "@mui/icons-material/Schedule";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { useTheme, type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { useQuery } from "react-query";
import { getTemplateVersion } from "api/api";
import type { TemplateVersion, Workspace } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Stack } from "components/Stack/Stack";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";

dayjs.extend(relativeTime);

type BatchUpdateConfirmationProps = {
  checkedWorkspaces: Workspace[];
  open: boolean;
  isLoading: boolean;
  onClose: () => void;
  onConfirm: () => void;
};

export const BatchUpdateConfirmation: FC<BatchUpdateConfirmationProps> = ({
  checkedWorkspaces,
  open,
  onClose,
  onConfirm,
  isLoading,
}) => {
  const workspaceCount = `${checkedWorkspaces.length} ${
    checkedWorkspaces.length === 1 ? "workspace" : "workspaces"
  }`;

  return (
    <ConfirmDialog
      type="delete"
      open={open}
      onClose={onClose}
      title={`Update ${workspaceCount}`}
      hideCancel
      confirmLoading={isLoading}
      confirmText={<>Update {workspaceCount}</>}
      onConfirm={onConfirm}
      description={<Workspaces workspaces={checkedWorkspaces} />}
    />
  );
};

const Workspaces: FC<{ workspaces: Workspace[] }> = ({ workspaces }) => {
  const mostRecent = workspaces.reduce(
    (latestSoFar, against) => {
      if (!latestSoFar) {
        return against;
      }

      return new Date(against.last_used_at).getTime() >
        new Date(latestSoFar.last_used_at).getTime()
        ? against
        : latestSoFar;
    },
    undefined as Workspace | undefined,
  );

  const workspacesCount = `${workspaces.length} ${
    workspaces.length === 1 ? "workspace" : "workspaces"
  }`;

  const newTemplateVersions = new Map(
    workspaces.map((it) => [
      it.template_active_version_id,
      it.template_display_name,
    ]),
  );
  const templatesCount = `${newTemplateVersions.size} ${
    newTemplateVersions.size === 1 ? "template" : "templates"
  }`;

  const { data, error } = useQuery({
    queryFn: () =>
      Promise.all(
        [...newTemplateVersions].map(
          async ([id, name]) => [name, await getTemplateVersion(id)] as const,
        ),
      ),
  });

  return (
    <>
      <TemplateVersionMessages templateVersions={data} error={error} />
      <Stack
        justifyContent="center"
        direction="row"
        wrap="wrap"
        css={{ gap: "6px 20px", fontSize: 14 }}
      >
        <Stack direction="row" alignItems="center" spacing={1}>
          <PersonIcon />
          <span>{workspacesCount}</span>
        </Stack>
        <Stack direction="row" alignItems="center" spacing={1}>
          <PersonIcon />
          <span>{templatesCount}</span>
        </Stack>
        {mostRecent && (
          <Stack direction="row" alignItems="center" spacing={1}>
            <ScheduleIcon css={styles.summaryIcon} />
            <span>Last used {dayjs(mostRecent.last_used_at).fromNow()}</span>
          </Stack>
        )}
      </Stack>
    </>
  );
};

interface TemplateVersionMessagesProps {
  error?: unknown;
  templateVersions?: Array<readonly [string, TemplateVersion]>;
}

const TemplateVersionMessages: FC<TemplateVersionMessagesProps> = ({
  error,
  templateVersions,
}) => {
  const theme = useTheme();

  if (error) {
    return <ErrorAlert error={error} />;
  }

  if (!templateVersions) {
    return <Loader />;
  }

  return (
    <ul css={styles.workspacesList}>
      {templateVersions.map(([templateName, version]) => (
        <li key={version.id} css={styles.workspace}>
          <Stack spacing={0}>
            <span css={{ fontWeight: 500, color: theme.experimental.l1.text }}>
              {templateName} ({version.name})
            </span>
            <span css={styles.message}>{version.message ?? "No message"}</span>
          </Stack>
        </li>
      ))}
    </ul>
  );
};

const PersonIcon: FC = () => {
  // This size doesn't match the rest of the icons because MUI is just really
  // inconsistent. We have to make it bigger than the rest, and pull things in
  // on the sides to compensate.
  return <PersonOutlinedIcon css={{ width: 18, height: 18, margin: -1 }} />;
};

const styles = {
  summaryIcon: { width: 16, height: 16 },

  consequences: {
    display: "flex",
    flexDirection: "column",
    gap: 8,
    paddingLeft: 16,
    marginBottom: 0,
  },

  workspacesList: (theme) => ({
    listStyleType: "none",
    padding: 0,
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    overflow: "hidden auto",
    maxHeight: 184,
  }),

  workspace: (theme) => ({
    padding: "8px 16px",
    borderBottom: `1px solid ${theme.palette.divider}`,

    "&:last-child": {
      border: "none",
    },
  }),

  message: {
    fontSize: 13,
  },
} satisfies Record<string, Interpolation<Theme>>;
