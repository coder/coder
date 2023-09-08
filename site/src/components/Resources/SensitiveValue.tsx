import IconButton from "@mui/material/IconButton";
import { makeStyles } from "@mui/styles";
import Tooltip from "@mui/material/Tooltip";
import VisibilityOffOutlined from "@mui/icons-material/VisibilityOffOutlined";
import VisibilityOutlined from "@mui/icons-material/VisibilityOutlined";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { useState } from "react";

const Language = {
  showLabel: "Show value",
  hideLabel: "Hide value",
};

export const SensitiveValue: React.FC<{ value: string }> = ({ value }) => {
  const [shouldDisplay, setShouldDisplay] = useState(false);
  const styles = useStyles();
  const displayValue = shouldDisplay ? value : "••••••••";
  const buttonLabel = shouldDisplay ? Language.hideLabel : Language.showLabel;
  const icon = shouldDisplay ? (
    <VisibilityOffOutlined />
  ) : (
    <VisibilityOutlined />
  );

  return (
    <div className={styles.sensitiveValue}>
      <CopyableValue value={value} className={styles.value}>
        {displayValue}
      </CopyableValue>
      <Tooltip title={buttonLabel}>
        <IconButton
          className={styles.button}
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

const useStyles = makeStyles((theme) => ({
  value: {
    // 22px is the button width
    width: "calc(100% - 22px)",
    overflow: "hidden",
    whiteSpace: "nowrap",
    textOverflow: "ellipsis",
  },

  sensitiveValue: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),
  },

  button: {
    color: "inherit",

    "& .MuiSvgIcon-root": {
      width: 16,
      height: 16,
    },
  },
}));
