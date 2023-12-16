import Link from "@mui/material/Link";
// This is used as base for the main HelpTooltip component
// eslint-disable-next-line no-restricted-imports -- Read above
import Popover, { type PopoverProps } from "@mui/material/Popover";
import HelpIcon from "@mui/icons-material/HelpOutline";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import {
  createContext,
  useContext,
  useRef,
  useState,
  type FC,
  type PropsWithChildren,
  type HTMLAttributes,
  type ReactNode,
} from "react";
import { Stack } from "components/Stack/Stack";
import type { CSSObject } from "@emotion/css";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type ClassName, useClassName } from "hooks/useClassName";

type Icon = typeof HelpIcon;

type Size = "small" | "medium";

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

interface HelpPopoverProps extends PopoverProps {
  onOpen: () => void;
  onClose: () => void;
}

export const HelpPopover: FC<HelpPopoverProps> = ({
  onOpen,
  onClose,
  children,
  ...props
}) => {
  const popover = useClassName(classNames.popover, []);
  const paper = useClassName(classNames.paper, []);

  return (
    <Popover
      className={popover}
      classes={{ paper }}
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

export interface HelpTooltipProps {
  // Useful to test on storybook
  open?: boolean;
  size?: Size;
  icon?: Icon;
  buttonStyles?: Interpolation<Theme>;
  iconStyles?: Interpolation<Theme>;
  children?: ReactNode;
}

export const HelpTooltip: FC<HelpTooltipProps> = ({
  children,
  open = false,
  size = "medium",
  icon: Icon = HelpIcon,
  buttonStyles,
  iconStyles,
}) => {
  const theme = useTheme();
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(open);
  const id = isOpen ? "help-popover" : undefined;

  const onClose = () => {
    setIsOpen(false);
  };

  return (
    <>
      <button
        ref={anchorRef}
        aria-describedby={id}
        css={[
          css`
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
          `,
          buttonStyles,
        ]}
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
          css={[
            {
              width: theme.spacing(getIconSpacingFromSize(size)),
              height: theme.spacing(getIconSpacingFromSize(size)),
            },
            iconStyles,
          ]}
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

export const HelpTooltipTitle: FC<HTMLAttributes<HTMLHeadingElement>> = ({
  children,
  ...attrs
}) => {
  return (
    <h4 css={styles.title} {...attrs}>
      {children}
    </h4>
  );
};

export const HelpTooltipText: FC<HTMLAttributes<HTMLParagraphElement>> = ({
  children,
  ...attrs
}) => {
  return (
    <p css={styles.text} {...attrs}>
      {children}
    </p>
  );
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

interface HelpTooltipActionProps {
  children?: ReactNode;
  icon: Icon;
  onClick: () => void;
  ariaLabel?: string;
}

export const HelpTooltipAction: FC<HelpTooltipActionProps> = ({
  children,
  icon: Icon,
  onClick,
  ariaLabel,
}) => {
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

export const HelpTooltipLinksGroup: FC<PropsWithChildren> = ({ children }) => {
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

const classNames = {
  popover: (css) => css`
    pointer-events: none;
  `,

  paper: (css, theme) => css`
    ${theme.typography.body2 as CSSObject}

    margin-top: 4px;
    width: 304px;
    padding: 20px;
    color: ${theme.palette.text.secondary};
    pointer-events: auto;
  `,
} satisfies Record<string, ClassName>;

const styles = {
  title: (theme) => ({
    marginTop: 0,
    marginBottom: 8,
    color: theme.palette.text.primary,
    fontSize: 14,
    lineHeight: "150%",
    fontWeight: 600,
  }),

  text: (theme) => ({
    marginTop: 4,
    marginBottom: 4,
    ...(theme.typography.body2 as CSSObject),
  }),

  link: (theme) => ({
    display: "flex",
    alignItems: "center",
    ...(theme.typography.body2 as CSSObject),
  }),

  linkIcon: {
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: 8,
  },

  linksGroup: {
    marginTop: 16,
  },

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

  actionIcon: {
    color: "inherit",
    width: 14,
    height: 14,
    marginRight: 8,
  },
} satisfies Record<string, Interpolation<Theme>>;
