import Link from "@mui/material/Link";
import Popover, { PopoverProps } from "@mui/material/Popover";
import { makeStyles } from "@mui/styles";
import HelpIcon from "@mui/icons-material/HelpOutline";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import {
  createContext,
  useContext,
  useRef,
  useState,
  FC,
  PropsWithChildren,
} from "react";
import { combineClasses } from "utils/combineClasses";
import { Stack } from "components/Stack/Stack";
import Box, { BoxProps } from "@mui/material/Box";

type Icon = typeof HelpIcon;

type Size = "small" | "medium";
export interface HelpTooltipProps {
  // Useful to test on storybook
  open?: boolean;
  size?: Size;
  icon?: Icon;
  iconClassName?: string;
  buttonClassName?: string;
}

export const HelpTooltipContext = createContext<
  { open: boolean; onClose: () => void } | undefined
>(undefined);

const useHelpTooltip = () => {
  const helpTooltipContext = useContext(HelpTooltipContext);

  if (!helpTooltipContext) {
    throw new Error(
      "This hook should be used in side of the HelpTooltipContext.",
    );
  }

  return helpTooltipContext;
};

export const HelpPopover: FC<
  PopoverProps & { onOpen: () => void; onClose: () => void }
> = ({ onOpen, onClose, children, ...props }) => {
  const styles = useStyles({ size: "small" });

  return (
    <Popover
      className={styles.popover}
      classes={{ paper: styles.popoverPaper }}
      onClose={onClose}
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "left",
      }}
      transformOrigin={{
        vertical: "top",
        horizontal: "left",
      }}
      PaperProps={{
        onMouseEnter: onOpen,
        onMouseLeave: onClose,
      }}
      {...props}
    >
      {children}
    </Popover>
  );
};

export const HelpTooltip: FC<PropsWithChildren<HelpTooltipProps>> = ({
  children,
  open,
  size = "medium",
  icon: Icon = HelpIcon,
  iconClassName,
  buttonClassName,
}) => {
  const styles = useStyles({ size });
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(Boolean(open));
  const id = isOpen ? "help-popover" : undefined;

  const onClose = () => {
    setIsOpen(false);
  };

  return (
    <>
      <button
        ref={anchorRef}
        aria-describedby={id}
        className={combineClasses([styles.button, buttonClassName])}
        onClick={(event) => {
          event.stopPropagation();
          setIsOpen(true);
        }}
        onMouseEnter={() => {
          setIsOpen(true);
        }}
        onMouseLeave={() => {
          setIsOpen(false);
        }}
        aria-label="More info"
      >
        <Icon className={combineClasses([styles.icon, iconClassName])} />
      </button>
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipContext.Provider value={{ open: isOpen, onClose }}>
          {children}
        </HelpTooltipContext.Provider>
      </HelpPopover>
    </>
  );
};

export const HelpTooltipTitle: FC<PropsWithChildren<unknown>> = ({
  children,
}) => {
  const styles = useStyles({});

  return <h4 className={styles.title}>{children}</h4>;
};

export const HelpTooltipText = (props: BoxProps) => {
  const styles = useStyles({});

  return (
    <Box
      component="p"
      {...props}
      className={combineClasses([styles.text, props.className])}
    />
  );
};

export const HelpTooltipLink: FC<PropsWithChildren<{ href: string }>> = ({
  children,
  href,
}) => {
  const styles = useStyles({});

  return (
    <Link href={href} target="_blank" rel="noreferrer" className={styles.link}>
      <OpenInNewIcon className={styles.linkIcon} />
      {children}
    </Link>
  );
};

export const HelpTooltipAction: FC<
  PropsWithChildren<{
    icon: Icon;
    onClick: () => void;
    ariaLabel?: string;
  }>
> = ({ children, icon: Icon, onClick, ariaLabel }) => {
  const styles = useStyles({});
  const tooltip = useHelpTooltip();

  return (
    <button
      aria-label={ariaLabel ?? ""}
      className={styles.action}
      onClick={(event) => {
        event.stopPropagation();
        onClick();
        tooltip.onClose();
      }}
    >
      <Icon className={styles.actionIcon} />
      {children}
    </button>
  );
};

export const HelpTooltipLinksGroup: FC<PropsWithChildren<unknown>> = ({
  children,
}) => {
  const styles = useStyles({});

  return (
    <Stack spacing={1} className={styles.linksGroup}>
      {children}
    </Stack>
  );
};

const getButtonSpacingFromSize = (size?: Size): number => {
  switch (size) {
    case "small":
      return 2.5;
    case "medium":
    default:
      return 3;
  }
};

const getIconSpacingFromSize = (size?: Size): number => {
  switch (size) {
    case "small":
      return 1.5;
    case "medium":
    default:
      return 2;
  }
};

const useStyles = makeStyles((theme) => ({
  button: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    width: ({ size }: { size?: Size }) =>
      theme.spacing(getButtonSpacingFromSize(size)),
    height: ({ size }: { size?: Size }) =>
      theme.spacing(getButtonSpacingFromSize(size)),
    padding: 0,
    border: 0,
    background: "transparent",
    color: theme.palette.text.primary,
    opacity: 0.5,
    cursor: "pointer",

    "&:hover": {
      opacity: 0.75,
    },
  },

  icon: {
    width: ({ size }: { size?: Size }) =>
      theme.spacing(getIconSpacingFromSize(size)),
    height: ({ size }: { size?: Size }) =>
      theme.spacing(getIconSpacingFromSize(size)),
  },

  popover: {
    pointerEvents: "none",
  },

  popoverPaper: {
    marginTop: theme.spacing(0.5),
    width: theme.spacing(38),
    padding: theme.spacing(2.5),
    color: theme.palette.text.secondary,
    pointerEvents: "auto",
    ...theme.typography.body2,
  },

  title: {
    marginTop: 0,
    marginBottom: theme.spacing(1),
    color: theme.palette.text.primary,
    fontSize: 14,
    lineHeight: "120%",
    fontWeight: 600,
  },

  text: {
    marginTop: theme.spacing(0.5),
    marginBottom: theme.spacing(0.5),
    ...theme.typography.body2,
  },

  link: {
    display: "flex",
    alignItems: "center",
    ...theme.typography.body2,
  },

  linkIcon: {
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: theme.spacing(1),
  },

  linksGroup: {
    marginTop: theme.spacing(2),
  },

  action: {
    display: "flex",
    alignItems: "center",
    background: "none",
    border: 0,
    color: theme.palette.primary.light,
    padding: 0,
    cursor: "pointer",
    fontSize: 14,
  },

  actionIcon: {
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: theme.spacing(1),
  },
}));
