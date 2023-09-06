import IconButton from "@mui/material/Button";
import { makeStyles } from "@mui/styles";
import Tooltip from "@mui/material/Tooltip";
import Check from "@mui/icons-material/Check";
import { useClipboard } from "hooks/useClipboard";
import { combineClasses } from "../../utils/combineClasses";
import { FileCopyIcon } from "../Icons/FileCopyIcon";

interface CopyButtonProps {
  text: string;
  ctaCopy?: string;
  wrapperClassName?: string;
  buttonClassName?: string;
  tooltipTitle?: string;
}

export const Language = {
  tooltipTitle: "Copy to clipboard",
  ariaLabel: "Copy to clipboard",
};

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton: React.FC<React.PropsWithChildren<CopyButtonProps>> = ({
  text,
  ctaCopy,
  wrapperClassName = "",
  buttonClassName = "",
  tooltipTitle = Language.tooltipTitle,
}) => {
  const styles = useStyles();
  const { isCopied, copy: copyToClipboard } = useClipboard(text);

  return (
    <Tooltip title={tooltipTitle} placement="top">
      <div
        className={combineClasses([styles.copyButtonWrapper, wrapperClassName])}
      >
        <IconButton
          className={combineClasses([styles.copyButton, buttonClassName])}
          onClick={copyToClipboard}
          size="small"
          aria-label={Language.ariaLabel}
          variant="text"
        >
          {isCopied ? (
            <Check className={styles.fileCopyIcon} />
          ) : (
            <FileCopyIcon className={styles.fileCopyIcon} />
          )}
          {ctaCopy && <div className={styles.buttonCopy}>{ctaCopy}</div>}
        </IconButton>
      </div>
    </Tooltip>
  );
};

const useStyles = makeStyles((theme) => ({
  copyButtonWrapper: {
    display: "flex",
  },
  copyButton: {
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(0.85),
    minWidth: 32,

    "&:hover": {
      background: theme.palette.background.paper,
    },
  },
  fileCopyIcon: {
    width: 20,
    height: 20,
  },
  buttonCopy: {
    marginLeft: theme.spacing(1),
  },
}));
