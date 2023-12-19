import Link from "@mui/material/Link";
import HelpIcon from "@mui/icons-material/HelpOutline";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import {
  type FC,
  type PropsWithChildren,
  type HTMLAttributes,
  type ReactNode,
  forwardRef,
  ComponentProps,
} from "react";
import { Stack } from "components/Stack/Stack";
import { type CSSObject } from "@emotion/css";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";

type Icon = typeof HelpIcon;

type Size = "small" | "medium";

export const HelpTooltipIcon = HelpIcon;

export const HelpTooltip: FC<ComponentProps<typeof Popover>> = (props) => {
  return <Popover mode="hover" {...props} />;
};

export const HelpTooltipContent = (
  props: ComponentProps<typeof PopoverContent>,
) => {
  const theme = useTheme();

  return (
    <PopoverContent
      {...props}
      css={{
        "& .MuiPaper-root": {
          fontSize: 14,
          width: 304,
          padding: 20,
          color: theme.palette.text.secondary,
        },
      }}
    />
  );
};

type HelpTooltipTriggerProps = HTMLAttributes<HTMLButtonElement> & {
  size?: Size;
  hoverEffect?: boolean;
};

export const HelpTooltipTrigger = forwardRef<
  HTMLButtonElement,
  HelpTooltipTriggerProps
>((props, ref) => {
  const {
    size = "medium",
    children = <HelpTooltipIcon />,
    hoverEffect = true,
    ...buttonProps
  } = props;

  const hoverEffectStyles = css({
    opacity: 0.5,
    "&:hover": {
      opacity: 0.75,
    },
  });

  return (
    <PopoverTrigger>
      <button
        {...buttonProps}
        aria-label="More info"
        ref={ref}
        css={[
          css`
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 4px 0;
            border: 0;
            background: transparent;
            cursor: pointer;
            color: inherit;

            & svg {
              width: ${getIconSpacingFromSize(size)}px;
              height: ${getIconSpacingFromSize(size)}px;
            }
          `,
          hoverEffect ? hoverEffectStyles : null,
        ]}
      >
        {children}
      </button>
    </PopoverTrigger>
  );
});

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
  const popover = usePopover();

  return (
    <button
      aria-label={ariaLabel ?? ""}
      css={styles.action}
      onClick={(event) => {
        event.stopPropagation();
        onClick();
        popover.setIsOpen(false);
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

const getIconSpacingFromSize = (size?: Size): number => {
  switch (size) {
    case "small":
      return 12;
    case "medium":
    default:
      return 16;
  }
};

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
