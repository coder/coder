import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import { Workspace } from "api/typesGenerated"
import { FC, ReactNode } from "react"
import { TableEmpty } from "components/TableEmpty/TableEmpty"
import { useTranslation } from "react-i18next"
import { TableLoaderSkeleton } from "components/TableLoader/TableLoader"
import AddOutlined from "@mui/icons-material/AddOutlined"
import Button from "@mui/material/Button"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Link as RouterLink, useNavigate } from "react-router-dom"
import { makeStyles } from "@mui/styles"
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip"
import InfoIcon from "@mui/icons-material/InfoOutlined"
import { colors } from "theme/colors"
import { useClickableTableRow } from "hooks/useClickableTableRow"
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight"
import Box from "@mui/material/Box"
import { AvatarData } from "components/AvatarData/AvatarData"
import { Avatar } from "components/Avatar/Avatar"
import { Stack } from "components/Stack/Stack"
import { LastUsed } from "components/LastUsed/LastUsed"
import { WorkspaceOutdatedTooltip } from "components/Tooltips"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { getDisplayWorkspaceTemplateName } from "utils/workspace"

export interface WorkspacesTableProps {
  workspaces?: Workspace[]
  isUsingFilter: boolean
  onUpdateWorkspace: (workspace: Workspace) => void
  error?: unknown
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({
  workspaces,
  isUsingFilter,
  onUpdateWorkspace,
}) => {
  const { t } = useTranslation("workspacesPage")
  const styles = useStyles()

  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="40%">Name</TableCell>
            <TableCell width="25%">Template</TableCell>
            <TableCell width="20%">Last used</TableCell>
            <TableCell width="15%">Status</TableCell>
            <TableCell width="1%" />
          </TableRow>
        </TableHead>
        <TableBody>
          {!workspaces && <TableLoaderSkeleton columns={5} useAvatarData />}
          {workspaces && workspaces.length === 0 && (
            <ChooseOne>
              <Cond condition={isUsingFilter}>
                <TableEmpty message={t("emptyResultsMessage")} />
              </Cond>

              <Cond>
                <TableEmpty
                  className={styles.withImage}
                  message={t("emptyCreateWorkspaceMessage")}
                  description={t("emptyCreateWorkspaceDescription")}
                  cta={
                    <Button
                      component={RouterLink}
                      to="/templates"
                      startIcon={<AddOutlined />}
                      variant="contained"
                    >
                      {t("createFromTemplateButton")}
                    </Button>
                  }
                  image={
                    <div className={styles.emptyImage}>
                      <img src="/featured/workspaces.webp" alt="" />
                    </div>
                  }
                />
              </Cond>
            </ChooseOne>
          )}
          {workspaces &&
            workspaces.map((workspace) => (
              <WorkspacesRow workspace={workspace} key={workspace.id}>
                <TableCell>
                  <AvatarData
                    title={
                      <Stack direction="row" spacing={0} alignItems="center">
                        {workspace.name}
                        {workspace.outdated && (
                          <WorkspaceOutdatedTooltip
                            templateName={workspace.template_name}
                            templateId={workspace.template_id}
                            onUpdateVersion={() => {
                              onUpdateWorkspace(workspace)
                            }}
                          />
                        )}
                      </Stack>
                    }
                    subtitle={workspace.owner_name}
                    avatar={
                      <Avatar
                        src={workspace.template_icon}
                        variant={workspace.template_icon ? "square" : undefined}
                        fitImage={Boolean(workspace.template_icon)}
                      >
                        {workspace.name}
                      </Avatar>
                    }
                  />
                </TableCell>

                <TableCell>
                  {getDisplayWorkspaceTemplateName(workspace)}
                </TableCell>

                <TableCell>
                  <LastUsed lastUsedAt={workspace.last_used_at} />
                </TableCell>

                <TableCell>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <WorkspaceStatusBadge workspace={workspace} />
                    {workspace.latest_build.status === "running" &&
                      !workspace.health.healthy && <UnhealthyTooltip />}
                  </Box>
                </TableCell>

                <TableCell>
                  <Box
                    sx={{
                      display: "flex",
                      paddingLeft: (theme) => theme.spacing(2),
                    }}
                  >
                    <KeyboardArrowRight
                      sx={{
                        color: (theme) => theme.palette.text.secondary,
                        width: 20,
                        height: 20,
                      }}
                    />
                  </Box>
                </TableCell>
              </WorkspacesRow>
            ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

const WorkspacesRow: FC<{
  workspace: Workspace
  children: ReactNode
}> = ({ workspace, children }) => {
  const navigate = useNavigate()
  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`
  const clickable = useClickableTableRow(() => {
    navigate(workspacePageLink)
  })

  return (
    <TableRow data-testid={`workspace-${workspace.id}`} {...clickable}>
      {children}
    </TableRow>
  )
}

export const UnhealthyTooltip = () => {
  const styles = useUnhealthyTooltipStyles()

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={styles.unhealthyIcon}
      buttonClassName={styles.unhealthyButton}
    >
      <HelpTooltipTitle>Workspace is unhealthy</HelpTooltipTitle>
      <HelpTooltipText>
        Your workspace is running but some agents are unhealthy.
      </HelpTooltipText>
    </HelpTooltip>
  )
}

const useUnhealthyTooltipStyles = makeStyles(() => ({
  unhealthyIcon: {
    color: colors.yellow[5],
  },

  unhealthyButton: {
    opacity: 1,

    "&:hover": {
      opacity: 1,
    },
  },
}))

const useStyles = makeStyles((theme) => ({
  withImage: {
    paddingBottom: 0,
  },
  emptyImage: {
    maxWidth: "50%",
    height: theme.spacing(34),
    overflow: "hidden",
    marginTop: theme.spacing(6),
    opacity: 0.85,

    "& img": {
      maxWidth: "100%",
    },
  },
}))
