import { makeStyles } from "@mui/styles";
import { TemplateExample } from "api/typesGenerated";
import { FC } from "react";
import { Link } from "react-router-dom";
import { combineClasses } from "utils/combineClasses";

export interface TemplateExampleCardProps {
  example: TemplateExample;
  className?: string;
}

export const TemplateExampleCard: FC<TemplateExampleCardProps> = ({
  example,
  className,
}) => {
  const styles = useStyles();

  return (
    <Link
      to={`/starter-templates/${example.id}`}
      className={combineClasses([styles.template, className])}
      key={example.id}
    >
      <div className={styles.templateIcon}>
        <img src={example.icon} alt="" />
      </div>
      <div className={styles.templateInfo}>
        <span className={styles.templateName}>{example.name}</span>
        <span className={styles.templateDescription}>
          {example.description}
        </span>
      </div>
    </Link>
  );
};

const useStyles = makeStyles((theme) => ({
  template: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    background: theme.palette.background.paper,
    textDecoration: "none",
    textAlign: "left",
    color: "inherit",
    display: "flex",
    alignItems: "center",
    height: "fit-content",

    "&:hover": {
      backgroundColor: theme.palette.background.paperLight,
    },
  },

  templateIcon: {
    width: theme.spacing(12),
    height: theme.spacing(12),
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    flexShrink: 0,

    "& img": {
      height: theme.spacing(4),
    },
  },

  templateInfo: {
    padding: theme.spacing(2, 2, 2, 0),
    display: "flex",
    flexDirection: "column",
    overflow: "hidden",
  },

  templateName: {
    fontSize: theme.spacing(2),
    textOverflow: "ellipsis",
    width: "100%",
    overflow: "hidden",
    whiteSpace: "nowrap",
  },

  templateDescription: {
    fontSize: theme.spacing(1.75),
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    width: "100%",
    overflow: "hidden",
    whiteSpace: "nowrap",
  },
}));
