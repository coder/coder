import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import React from "react"
import { maxWidth, sidePadding } from "../../theme/constants"
import { HeaderButton } from "../HeaderButton/HeaderButton"

export interface HeaderAction {
  readonly text: string
  readonly onClick?: (event: MouseEvent) => void
}

export interface HeaderProps {
  description?: string
  title: string
  subTitle?: string
  action?: HeaderAction
}

export const Header: React.FC<HeaderProps> = ({ description, title, subTitle, action }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.top}>
        <div className={styles.topInner}>
          <Box display="flex" flexDirection="column" minWidth={0}>
            <div>
              <Box display="flex" alignItems="center">
                <Typography variant="h3" className={styles.title}>
                  <Box component="span" maxWidth="100%" overflow="hidden" textOverflow="ellipsis">
                    {title}
                  </Box>
                </Typography>

                {subTitle && (
                  <div className={styles.subtitle}>
                    <Typography style={{ fontSize: 16 }}>{subTitle}</Typography>
                  </div>
                )}
              </Box>
              {description && (
                <Typography variant="caption" className={styles.description}>
                  {description}
                </Typography>
              )}
            </div>
          </Box>

          {action && (
            <>
              <div className={styles.actions}>
                <HeaderButton key={action.text} {...action} />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

const secondaryText = "#B5BFD2"
const useStyles = makeStyles((theme) => ({
  root: {},
  top: {
    position: "relative",
    display: "flex",
    alignItems: "center",
    height: 126,
    background: theme.palette.background.default,
    boxShadow: theme.shadows[3],
  },
  topInner: {
    display: "flex",
    alignItems: "center",
    maxWidth,
    margin: "0 auto",
    flex: 1,
    height: 68,
    minWidth: 0,
    padding: `0 ${sidePadding}`,
  },
  title: {
    display: "flex",
    alignItems: "center",
    fontWeight: "bold",
    whiteSpace: "nowrap",
    minWidth: 0,
    color: theme.palette.primary.contrastText,
  },
  description: {
    display: "block",
    marginTop: theme.spacing(1) / 2,
    marginBottom: -26,
    color: secondaryText,
  },
  subtitle: {
    position: "relative",
    top: 2,
    display: "flex",
    alignItems: "center",
    borderLeft: `1px solid ${theme.palette.divider}`,
    height: 28,
    marginLeft: 16,
    paddingLeft: 16,
    color: secondaryText,
  },
  actions: {
    paddingLeft: "50px",
    paddingRight: 0,
    flex: 1,
    display: "flex",
    flexDirection: "row",
    justifyContent: "flex-end",
    alignItems: "center",
  },
}))
