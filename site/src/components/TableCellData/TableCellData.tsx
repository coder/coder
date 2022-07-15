import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { Stack } from "../Stack/Stack"

interface StyleProps {
  highlight?: boolean
}

export const TableCellData: React.FC = ({ children }) => {
  return <Stack spacing={0}>{children}</Stack>
}

export const TableCellDataPrimary: React.FC<{ highlight?: boolean }> = ({
  children,
  highlight,
}) => {
  const styles = useStyles({ highlight })

  return <span className={styles.primary}>{children}</span>
}

export const TableCellDataSecondary: React.FC = ({ children }) => {
  const styles = useStyles()

  return <span className={styles.secondary}>{children}</span>
}

const useStyles = makeStyles((theme) => ({
  primary: {
    color: ({ highlight }: StyleProps) =>
      highlight ? theme.palette.text.primary : theme.palette.text.secondary,
    fontWeight: ({ highlight }: StyleProps) => (highlight ? 600 : undefined),
  },

  secondary: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },
}))
