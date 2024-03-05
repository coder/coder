import type { Interpolation, Theme } from "@emotion/react";
import Collapse from "@mui/material/Collapse";
import Link from "@mui/material/Link";
import type { FC, ReactNode } from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";

export interface ExpanderProps {
  expanded: boolean;
  setExpanded: (val: boolean) => void;
  children?: ReactNode;
}

export const Expander: FC<ExpanderProps> = ({
  expanded,
  setExpanded,
  children,
}) => {
  const toggleExpanded = () => setExpanded(!expanded);

  return (
    <>
      {!expanded && (
        <Link onClick={toggleExpanded} css={styles.expandLink}>
          <span css={styles.text}>
            Click here to learn more
            <DropdownArrow margin={false} />
          </span>
        </Link>
      )}
      <Collapse in={expanded}>
        <div css={styles.text}>{children}</div>
      </Collapse>
      {expanded && (
        <Link
          onClick={toggleExpanded}
          css={[styles.expandLink, styles.collapseLink]}
        >
          <span css={styles.text}>
            Click here to hide
            <DropdownArrow margin={false} close />
          </span>
        </Link>
      )}
    </>
  );
};

const styles = {
  expandLink: (theme) => ({
    cursor: "pointer",
    color: theme.palette.text.secondary,
  }),
  collapseLink: {
    marginTop: 16,
  },
  text: (theme) => ({
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontSize: theme.typography.caption.fontSize,
  }),
} satisfies Record<string, Interpolation<Theme>>;
