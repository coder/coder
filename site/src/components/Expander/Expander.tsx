import Collapse from "@mui/material/Collapse";
import Link from "@mui/material/Link";
import makeStyles from "@mui/styles/makeStyles";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { type FC, type PropsWithChildren } from "react";
import { combineClasses } from "utils/combineClasses";

export interface ExpanderProps {
  expanded: boolean;
  setExpanded: (val: boolean) => void;
}

export const Expander: FC<PropsWithChildren<ExpanderProps>> = ({
  expanded,
  setExpanded,
  children,
}) => {
  const styles = useStyles();

  const toggleExpanded = () => setExpanded(!expanded);

  return (
    <>
      {!expanded && (
        <Link onClick={toggleExpanded} className={styles.expandLink}>
          <span className={styles.text}>
            Click here to learn more
            <DropdownArrow margin={false} />
          </span>
        </Link>
      )}
      <Collapse in={expanded}>
        <div className={styles.text}>{children}</div>
      </Collapse>
      {expanded && (
        <Link
          onClick={toggleExpanded}
          className={combineClasses([styles.expandLink, styles.collapseLink])}
        >
          <span className={styles.text}>
            Click here to hide
            <DropdownArrow margin={false} close />
          </span>
        </Link>
      )}
    </>
  );
};

const useStyles = makeStyles((theme) => ({
  expandLink: {
    cursor: "pointer",
    color: theme.palette.text.secondary,
  },
  collapseLink: {
    marginTop: theme.spacing(2),
  },
  text: {
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontSize: theme.typography.caption.fontSize,
  },
}));
