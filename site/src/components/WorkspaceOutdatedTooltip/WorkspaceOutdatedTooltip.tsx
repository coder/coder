import RefreshIcon from "@mui/icons-material/Refresh"
import { FC } from "react"
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip"
import InfoIcon from "@mui/icons-material/InfoOutlined"
import { makeStyles } from "@mui/styles"
import { colors } from "theme/colors"
import { useQuery } from "@tanstack/react-query"
import { getTemplate, getTemplateVersion } from "api/api"
import Box from "@mui/material/Box"
import Skeleton from "@mui/material/Skeleton"
import Link from "@mui/material/Link"

export const Language = {
  outdatedLabel: "Outdated",
  versionTooltipText:
    "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update version",
}

interface TooltipProps {
  onUpdateVersion: () => void
  templateId: string
  templateName: string
  ariaLabel?: string
}

export const WorkspaceOutdatedTooltip: FC<TooltipProps> = ({
  onUpdateVersion,
  ariaLabel,
  templateId,
  templateName,
}) => {
  const styles = useStyles()
  const { data: activeVersion } = useQuery({
    queryFn: async () => {
      const template = await getTemplate(templateId)
      const activeVersion = await getTemplateVersion(template.active_version_id)
      return activeVersion
    },
    queryKey: ["templates", templateId, "activeVersion"],
  })

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={styles.icon}
      buttonClassName={styles.button}
    >
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>

      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          gap: 1,
          py: 1,
          fontSize: 13,
        }}
      >
        <Box>
          <Box
            sx={{
              color: (theme) => theme.palette.text.primary,
              fontWeight: 600,
            }}
          >
            New version
          </Box>
          <Box>
            {activeVersion ? (
              <Link
                href={`/templates/${templateName}/versions/${activeVersion.name}`}
                target="_blank"
                sx={{ color: (theme) => theme.palette.primary.light }}
              >
                {activeVersion.name}
              </Link>
            ) : (
              <Skeleton variant="text" height={20} width={100} />
            )}
          </Box>
        </Box>

        <Box>
          <Box
            sx={{
              color: (theme) => theme.palette.text.primary,
              fontWeight: 600,
            }}
          >
            Message
          </Box>
          <Box>
            {activeVersion ? (
              activeVersion.message === "" ? (
                "No message"
              ) : (
                activeVersion.message
              )
            ) : (
              <Skeleton variant="text" height={20} width={150} />
            )}
          </Box>
        </Box>
      </Box>

      <HelpTooltipLinksGroup>
        <HelpTooltipAction
          icon={RefreshIcon}
          onClick={onUpdateVersion}
          ariaLabel={ariaLabel}
        >
          {Language.updateVersionLabel}
        </HelpTooltipAction>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}

const useStyles = makeStyles(() => ({
  icon: {
    color: colors.yellow[5],
  },

  button: {
    opacity: 1,

    "&:hover": {
      opacity: 1,
    },
  },
}))
