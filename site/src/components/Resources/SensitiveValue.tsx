import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import VisibilityOffOutlined from "@mui/icons-material/VisibilityOffOutlined";
import VisibilityOutlined from "@mui/icons-material/VisibilityOutlined";
import { useState } from "react";
import { css } from "@emotion/react";
import { CopyableValue } from "components/CopyableValue/CopyableValue";

const Language = {
  showLabel: "Show value",
  hideLabel: "Hide value",
};

export const SensitiveValue: React.FC<{ value: string }> = ({ value }) => {
  const [shouldDisplay, setShouldDisplay] = useState(false);
  const displayValue = shouldDisplay ? value : "••••••••";
  const buttonLabel = shouldDisplay ? Language.hideLabel : Language.showLabel;
  const icon = shouldDisplay ? (
    <VisibilityOffOutlined />
  ) : (
    <VisibilityOutlined />
  );

  return (
    <div
      css={(theme) => ({
        display: "flex",
        alignItems: "center",
        gap: theme.spacing(0.5),
      })}
    >
      <CopyableValue
        value={value}
        css={{
          // 22px is the button width
          width: "calc(100% - 22px)",
          overflow: "hidden",
          whiteSpace: "nowrap",
          textOverflow: "ellipsis",
        }}
      >
        {displayValue}
      </CopyableValue>
      <Tooltip title={buttonLabel}>
        <IconButton
          css={css`
            color: inherit;

            & .MuiSvgIcon-root {
              width: 16px;
              height: 16px;
            }
          `}
          onClick={() => {
            setShouldDisplay((value) => !value);
          }}
          size="small"
          aria-label={buttonLabel}
        >
          {icon}
        </IconButton>
      </Tooltip>
    </div>
  );
};
