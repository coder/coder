import Link from "@mui/material/Link";
import Popover, { PopoverProps } from "@mui/material/Popover";
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
import { Stack } from "components/Stack/Stack";
import Box, { BoxProps } from "@mui/material/Box";
import { type CSSObject, css as className } from "@emotion/css";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";

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
  const theme = useTheme();

  return (
    <Popover
      className={className`
        pointer-events: none;
      `}
      classes={{
        paper: className`
          ${theme.typography.body2 as CSSObject}

          margin-top: ${theme.spacing(0.5)};
          width: ${theme.spacing(38)};
          padding: ${theme.spacing(2.5)};
          color: ${theme.palette.text.secondary};
          pointer-events: auto;
        `,
      }}
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
  const theme = useTheme();
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
        css={css`
          display: flex;
          align-items: center;
          justify-content: center;
          width: ${theme.spacing(getButtonSpacingFromSize(size))};
          height: ${theme.spacing(getButtonSpacingFromSize(size))};
          padding: 0;
          border: 0;
          background: transparent;
          color: ${theme.palette.text.primary};
          opacity: 0.5;
          cursor: pointer;

          &:hover {
            opacity: 0.75;
          }
        `}
        className={buttonClassName}
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
        <Icon
          css={{
            width: theme.spacing(getIconSpacingFromSize(size)),
            height: theme.spacing(getIconSpacingFromSize(size)),
          }}
          className={iconClassName}
        />
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
  return <h4 css={styles.title}>{children}</h4>;
};

export const HelpTooltipText = (props: BoxProps) => {
  return <Box component="p" css={styles.text} {...props} />;
};

export const HelpTooltipLink: FC<PropsWithChildren<{ href: string }>> = ({
  children,
  href,
}) => {
  return (
    <Link href={href} target="_blank" rel="noreferrer" css={styles.link}>
      <OpenInNewIcon css={styles.linkIcon} />
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
  const tooltip = useHelpTooltip();

  return (
    <button
      aria-label={ariaLabel ?? ""}
      css={styles.action}
      onClick={(event) => {
        event.stopPropagation();
        onClick();
        tooltip.onClose();
      }}
    >
      <Icon css={styles.actionIcon} />
      {children}
    </button>
  );
};

export const HelpTooltipLinksGroup: FC<PropsWithChildren<unknown>> = ({
  children,
}) => {
  return (
    <Stack spacing={1} css={styles.linksGroup}>
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

const styles = {
  title: (theme) => ({
    marginTop: 0,
    marginBottom: theme.spacing(1),
    color: theme.palette.text.primary,
    fontSize: 14,
    lineHeight: "120%",
    fontWeight: 600,
  }),

  text: (theme) => ({
    marginTop: theme.spacing(0.5),
    marginBottom: theme.spacing(0.5),
    ...(theme.typography.body2 as CSSObject),
  }),

  link: (theme) => ({
    display: "flex",
    alignItems: "center",
    ...(theme.typography.body2 as CSSObject),
  }),

  linkIcon: (theme) => ({
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: theme.spacing(1),
  }),

  linksGroup: (theme) => ({
    marginTop: theme.spacing(2),
  }),

  action: (theme) => ({
    display: "flex",
    alignItems: "center",
    background: "none",
    border: 0,
    color: theme.palette.primary.light,
    padding: 0,
    cursor: "pointer",
    fontSize: 14,
  }),

  actionIcon: (theme) => ({
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: theme.spacing(1),
  }),
} satisfies Record<string, Interpolation<Theme>>;
