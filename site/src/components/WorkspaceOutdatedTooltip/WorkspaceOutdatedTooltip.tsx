import RefreshIcon from "@mui/icons-material/Refresh";
import { type FC } from "react";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import { useQuery } from "react-query";
import Box from "@mui/material/Box";
import Skeleton from "@mui/material/Skeleton";
import Link from "@mui/material/Link";
import { css } from "@emotion/css";
import { templateVersion } from "api/queries/templates";
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { colors } from "theme/colors";

export const Language = {
  outdatedLabel: "Outdated",
  versionTooltipText:
    "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update",
};

interface TooltipProps {
  onUpdateVersion: () => void;
  latestVersionId: string;
  templateName: string;
  ariaLabel?: string;
}

export const WorkspaceOutdatedTooltip: FC<TooltipProps> = ({
  onUpdateVersion,
  ariaLabel,
  latestVersionId,
  templateName,
}) => {
  const { data: activeVersion } = useQuery(templateVersion(latestVersionId));

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={css`
        color: ${colors.yellow[5]};
      `}
      buttonClassName={css`
        opacity: 1;

        &:hover {
          opacity: 1;
        }
      `}
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
  );
};
